#!/usr/bin/env python3
"""Coordinate → Cadastral Code bridge.

Resolves lat/lon coordinates to Georgian cadastral codes using ArcGIS MapServer.
Primary use: link Place.ge listings (which have coordinates but no cadastral codes)
to NAPR ownership data (which requires cadastral codes).

Pipeline: listing coords → THIS SCRIPT → UNIQ_CODE → napr_lookup.py → ownership

Backend: ArcGIS CadRepGeo MapServer Layer 14 (HTTP only).
Note: GeoServer WFS on nv.napr.gov.ge is disabled (returns ServiceException).

Usage:
    # Single point lookup
    python3 tools/scripts/wfs_cadastre.py 41.7732 41.7286

    # Batch from Place.ge or SS.ge JSON
    python3 tools/scripts/wfs_cadastre.py --batch work/data/place_ge_coastal.json

    # Batch from CSV (lat,lon per line)
    python3 tools/scripts/wfs_cadastre.py --batch-csv coords.csv

    # Output as JSON
    python3 tools/scripts/wfs_cadastre.py 41.7732 41.7286 --json

    # Wider search radius (default 100m)
    python3 tools/scripts/wfs_cadastre.py 41.7732 41.7286 --radius 200
"""
from __future__ import annotations

import argparse
import json
import math
import sys
import time
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any

# ArcGIS endpoint — same as arcgis_spatial.py
ARCGIS_URL = (
    "http://gisappsn.reestri.gov.ge/ArcGIS/rest/services"
    "/CadRepGeo/MapServer/14/query"
)

# Rate limiting
REQUEST_DELAY = 0.3  # seconds between requests
DEFAULT_RADIUS_M = 100  # search radius in meters


def _meters_to_degrees(meters: float, lat: float) -> tuple[float, float]:
    """Convert meters to approximate lat/lon degrees at given latitude."""
    dlat = meters / 111_320
    dlon = meters / (111_320 * math.cos(math.radians(lat)))
    return dlat, dlon


def arcgis_point_query(
    lat: float, lon: float, radius_m: float = DEFAULT_RADIUS_M
) -> list[dict[str, Any]]:
    """Query ArcGIS for cadastral parcels near a point.

    Creates a small bbox around the point and queries Layer 14.
    Returns list of feature attributes (no geometry, to save bandwidth).
    """
    dlat, dlon = _meters_to_degrees(radius_m, lat)

    params = urllib.parse.urlencode({
        "where": "1=1",
        "geometry": json.dumps({
            "xmin": lon - dlon,
            "ymin": lat - dlat,
            "xmax": lon + dlon,
            "ymax": lat + dlat,
            "spatialReference": {"wkid": 4326},
        }),
        "geometryType": "esriGeometryEnvelope",
        "inSR": "4326",
        "spatialRel": "esriSpatialRelIntersects",
        "outFields": "*",
        "returnGeometry": "false",
        "outSR": "4326",
        "f": "json",
    })

    url = f"{ARCGIS_URL}?{params}"
    req = urllib.request.Request(url, headers={"User-Agent": "plot-bot/1.0"})

    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            data = json.loads(resp.read().decode("utf-8"))
            return data.get("features", [])
    except Exception as e:
        print(f"  ArcGIS error: {e}", file=sys.stderr)
        return []


def uniq_to_cadastral(uniq: str) -> str:
    """Convert 9-digit UNIQ_CODE to dotted cadastral format.

    053212220 → 05.32.12.220
    """
    u = str(uniq).zfill(9)
    return f"{u[:2]}.{u[2:4]}.{u[4:6]}.{u[6:9]}"


def lookup_point(
    lat: float, lon: float, radius_m: float = DEFAULT_RADIUS_M
) -> dict[str, Any]:
    """Resolve coordinates to cadastral code.

    Returns dict with cadastral_code, uniq_code, area_m2, and raw properties.
    If multiple parcels found, picks the one with smallest area (most specific).
    """
    result: dict[str, Any] = {
        "lat": lat,
        "lon": lon,
        "cadastral_code": None,
        "uniq_code": None,
        "area_m2": None,
        "parcel_count": 0,
        "properties": {},
        "source": "arcgis_layer14",
    }

    features = arcgis_point_query(lat, lon, radius_m)
    result["parcel_count"] = len(features)

    if not features:
        result["source"] = "none"
        return result

    # Pick best match: smallest area parcel (most specific to the point)
    best = None
    best_area = float("inf")
    for f in features:
        attrs = f.get("attributes", {})
        area = attrs.get("SHAPE_Area") or attrs.get("SHAPE.STArea()") or float("inf")
        try:
            area = float(area)
        except (ValueError, TypeError):
            area = float("inf")
        if area < best_area:
            best_area = area
            best = attrs

    if best:
        result["properties"] = best

        # Extract UNIQ_CODE
        uniq = best.get("UNIQ_CODE") or best.get("Uniq_Code") or best.get("uniq_code")
        if uniq:
            uniq = str(int(float(uniq)))  # handle numeric types
            result["uniq_code"] = uniq
            result["cadastral_code"] = uniq_to_cadastral(uniq)

        # Extract area (SHAPE_Area is in projection units, need to convert)
        area = best.get("SHAPE_Area") or best.get("SHAPE.STArea()")
        if area:
            try:
                result["area_m2"] = round(float(area), 1)
            except (ValueError, TypeError):
                pass

    return result


def batch_from_json(path: str, radius_m: float = DEFAULT_RADIUS_M) -> list[dict[str, Any]]:
    """Process JSON listings (Place.ge or SS.ge format)."""
    with open(path) as f:
        data = json.load(f)

    # Handle various JSON structures
    if isinstance(data, list):
        listings = data
    elif isinstance(data, dict):
        listings = data.get("listings", data.get("data", [data]))
    else:
        listings = [data]

    results = []
    for i, item in enumerate(listings):
        # Extract coordinates — try multiple field patterns
        lat = lon = None

        # Direct fields
        for lat_key in ("lat", "latitude", "y"):
            if lat_key in item and item[lat_key]:
                lat = float(item[lat_key])
                break
        for lon_key in ("lon", "lng", "longitude", "x"):
            if lon_key in item and item[lon_key]:
                lon = float(item[lon_key])
                break

        # Nested location object
        if lat is None or lon is None:
            loc = item.get("location", {})
            if isinstance(loc, dict):
                for lat_key in ("lat", "latitude"):
                    if lat_key in loc and loc[lat_key]:
                        lat = float(loc[lat_key])
                        break
                for lon_key in ("lon", "lng", "longitude"):
                    if lon_key in loc and loc[lon_key]:
                        lon = float(loc[lon_key])
                        break

        if lat is None or lon is None:
            print(f"  [{i+1}/{len(listings)}] SKIP — no coordinates", file=sys.stderr)
            continue

        # Skip if listing already has cadastral code
        existing_cad = item.get("cadastral_code") or item.get("cadcode")
        if existing_cad:
            results.append({
                "lat": lat, "lon": lon,
                "cadastral_code": existing_cad,
                "uniq_code": None,
                "area_m2": None,
                "parcel_count": -1,
                "properties": {},
                "source": "listing_field",
                "listing_id": item.get("id") or i,
                "listing_price": item.get("price") or item.get("price_total_usd"),
            })
            continue

        print(f"  [{i+1}/{len(listings)}] ({lat:.5f}, {lon:.5f})...", file=sys.stderr)
        result = lookup_point(lat, lon, radius_m)
        result["listing_id"] = item.get("id") or item.get("ID") or i
        result["listing_price"] = item.get("price") or item.get("Price") or item.get("price_total_usd")
        results.append(result)

        time.sleep(REQUEST_DELAY)

    return results


def batch_from_csv(path: str, radius_m: float = DEFAULT_RADIUS_M) -> list[dict[str, Any]]:
    """Process CSV with lat,lon per line."""
    results = []
    with open(path) as f:
        for i, line in enumerate(f):
            line = line.strip()
            if not line or line.startswith("#") or line.lower().startswith("lat"):
                continue
            parts = line.split(",")
            if len(parts) < 2:
                continue
            lat, lon = float(parts[0].strip()), float(parts[1].strip())
            print(f"  [{i+1}] ({lat:.5f}, {lon:.5f})...", file=sys.stderr)
            result = lookup_point(lat, lon, radius_m)
            results.append(result)
            time.sleep(REQUEST_DELAY)
    return results


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Coordinate → cadastral code (via ArcGIS MapServer)"
    )
    parser.add_argument("lat", nargs="?", type=float, help="Latitude (WGS84)")
    parser.add_argument("lon", nargs="?", type=float, help="Longitude (WGS84)")
    parser.add_argument("--radius", type=float, default=DEFAULT_RADIUS_M,
                        help=f"Search radius in meters (default: {DEFAULT_RADIUS_M})")
    parser.add_argument("--batch", help="Batch from JSON file (Place.ge/SS.ge)")
    parser.add_argument("--batch-csv", help="Batch from CSV (lat,lon per line)")
    parser.add_argument("--json", action="store_true", help="Output as JSON")
    parser.add_argument("--output", "-o", help="Write results to file")
    args = parser.parse_args()

    if args.batch:
        print(f"Batch: {args.batch} (radius={args.radius}m)...", file=sys.stderr)
        results = batch_from_json(args.batch, args.radius)
    elif args.batch_csv:
        print(f"Batch CSV: {args.batch_csv}...", file=sys.stderr)
        results = batch_from_csv(args.batch_csv, args.radius)
    elif args.lat is not None and args.lon is not None:
        results = [lookup_point(args.lat, args.lon, args.radius)]
    else:
        parser.print_help()
        sys.exit(1)

    # Format output
    if args.json or args.output:
        text = json.dumps(results, indent=2, ensure_ascii=False)
    else:
        lines = []
        for r in results:
            lid = r.get("listing_id", "")
            prefix = f"[{lid}] " if lid else ""
            cad = r.get("cadastral_code") or "NOT FOUND"
            uniq = r.get("uniq_code") or ""
            area = f"{r['area_m2']:.0f}m²" if r.get("area_m2") else "?"
            n = r.get("parcel_count", 0)
            src = r.get("source", "?")
            lines.append(f"{prefix}({r['lat']:.5f}, {r['lon']:.5f}) → {cad} (UNIQ:{uniq})  area={area}  parcels={n}  [{src}]")
        text = "\n".join(lines) + "\n" if lines else ""

    if args.output:
        Path(args.output).write_text(text)
        print(f"Written to {args.output}", file=sys.stderr)
    else:
        print(text, end="")

    # Summary
    resolved = sum(1 for r in results if r.get("cadastral_code"))
    from_listing = sum(1 for r in results if r.get("source") == "listing_field")
    from_arcgis = resolved - from_listing
    print(f"\n{resolved}/{len(results)} resolved ({from_arcgis} via ArcGIS, {from_listing} from listing field).", file=sys.stderr)


if __name__ == "__main__":
    main()
