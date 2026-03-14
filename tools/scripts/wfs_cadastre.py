#!/usr/bin/env python3
"""WFS Cadastre Lookup — coordinate → cadastral code via GeoServer.

Bridges the gap between listings (lat/lon) and NAPR (cadastral codes).
Queries nv.napr.gov.ge GeoServer WFS for cadastral features at a point.

Usage:
    # Single point lookup
    python3 tools/scripts/wfs_cadastre.py 41.7732 41.7286

    # Batch from JSON (Place.ge listings with lat/lon)
    python3 tools/scripts/wfs_cadastre.py --batch work/data/place_ge_coastal.json

    # Batch from CSV (lat,lon per line)
    python3 tools/scripts/wfs_cadastre.py --batch-csv coords.csv

    # Output as JSON
    python3 tools/scripts/wfs_cadastre.py 41.7732 41.7286 --json

    # Discover available layers
    python3 tools/scripts/wfs_cadastre.py --layers
"""
from __future__ import annotations

import argparse
import json
import sys
import time
import urllib.request
import urllib.parse
import xml.etree.ElementTree as ET
from pathlib import Path
from typing import Any

# GeoServer endpoints (HTTP only — SSL cert issues on HTTPS)
WFS_BASE = "http://nv.napr.gov.ge/geoserver/wfs"
WMS_BASE = "http://nv.napr.gov.ge/geoserver/wms"

# Known cadastral layers (discover more with --layers)
# Common GeoServer workspace:layer patterns for Georgian cadastre
CANDIDATE_LAYERS = [
    "napr:cadastre",
    "napr:parcels",
    "napr:land_parcels",
    "cadastre:parcels",
    "cadastre:land",
    "CadRepGeo:parcels",
    "public:cadastre",
]

# Rate limiting
REQUEST_DELAY = 0.5  # seconds between requests


def wfs_get_capabilities() -> str:
    """Fetch WFS GetCapabilities XML."""
    params = urllib.parse.urlencode({
        "service": "WFS",
        "version": "2.0.0",
        "request": "GetCapabilities",
    })
    url = f"{WFS_BASE}?{params}"
    req = urllib.request.Request(url, headers={"User-Agent": "plot-bot/1.0"})
    with urllib.request.urlopen(req, timeout=15) as resp:
        return resp.read().decode("utf-8")


def discover_layers() -> list[dict[str, str]]:
    """Discover available WFS feature types (layers)."""
    xml_text = wfs_get_capabilities()
    root = ET.fromstring(xml_text)

    # Handle WFS 2.0 and 1.1 namespaces
    ns = {
        "wfs": "http://www.opengis.net/wfs/2.0",
        "wfs11": "http://www.opengis.net/wfs",
        "ows": "http://www.opengis.net/ows/1.1",
    }

    layers = []

    # Try WFS 2.0 format
    for ft in root.findall(".//wfs:FeatureType", ns):
        name = ft.findtext("wfs:Name", "", ns) or ft.findtext("Name", "")
        title = ft.findtext("wfs:Title", "", ns) or ft.findtext("Title", "")
        if name:
            layers.append({"name": name, "title": title})

    # Try WFS 1.1 format if nothing found
    if not layers:
        for ft in root.findall(".//{http://www.opengis.net/wfs}FeatureType"):
            name = ft.findtext("{http://www.opengis.net/wfs}Name", "")
            title = ft.findtext("{http://www.opengis.net/wfs}Title", "")
            if name:
                layers.append({"name": name, "title": title})

    # Fallback: try without namespaces
    if not layers:
        for ft in root.iter("FeatureType"):
            name = ft.findtext("Name", "")
            title = ft.findtext("Title", "")
            if name:
                layers.append({"name": name, "title": title})

    return layers


def wfs_point_query(lat: float, lon: float, layer: str) -> list[dict[str, Any]]:
    """Query WFS for features intersecting a point."""
    # CQL_FILTER with INTERSECTS on a point geometry
    cql = f"INTERSECTS(geom, POINT({lon} {lat}))"

    params = urllib.parse.urlencode({
        "service": "WFS",
        "version": "2.0.0",
        "request": "GetFeature",
        "typeName": layer,
        "outputFormat": "application/json",
        "CQL_FILTER": cql,
        "count": "5",
    })
    url = f"{WFS_BASE}?{params}"
    req = urllib.request.Request(url, headers={"User-Agent": "plot-bot/1.0"})

    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            data = json.loads(resp.read().decode("utf-8"))
            return data.get("features", [])
    except Exception:
        return []


def wfs_bbox_query(lat: float, lon: float, layer: str, buffer: float = 0.001) -> list[dict[str, Any]]:
    """Query WFS with bounding box around a point (fallback if CQL fails)."""
    bbox = f"{lon - buffer},{lat - buffer},{lon + buffer},{lat + buffer}"

    params = urllib.parse.urlencode({
        "service": "WFS",
        "version": "2.0.0",
        "request": "GetFeature",
        "typeName": layer,
        "outputFormat": "application/json",
        "bbox": bbox,
        "count": "10",
    })
    url = f"{WFS_BASE}?{params}"
    req = urllib.request.Request(url, headers={"User-Agent": "plot-bot/1.0"})

    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            data = json.loads(resp.read().decode("utf-8"))
            return data.get("features", [])
    except Exception:
        return []


def wms_getfeatureinfo(lat: float, lon: float, layer: str) -> dict[str, Any] | None:
    """WMS GetFeatureInfo as fallback — click on map at point."""
    # Simulate a 256x256 tile centered on the point
    delta = 0.005
    bbox = f"{lon - delta},{lat - delta},{lon + delta},{lat + delta}"

    params = urllib.parse.urlencode({
        "service": "WMS",
        "version": "1.1.1",
        "request": "GetFeatureInfo",
        "layers": layer,
        "query_layers": layer,
        "info_format": "application/json",
        "srs": "EPSG:4326",
        "bbox": bbox,
        "width": "256",
        "height": "256",
        "x": "128",  # center
        "y": "128",
        "feature_count": "5",
    })
    url = f"{WMS_BASE}?{params}"
    req = urllib.request.Request(url, headers={"User-Agent": "plot-bot/1.0"})

    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            data = json.loads(resp.read().decode("utf-8"))
            features = data.get("features", [])
            if features:
                return features[0].get("properties", {})
    except Exception:
        pass
    return None


def lookup_point(lat: float, lon: float, layer: str | None = None) -> dict[str, Any]:
    """Full lookup pipeline for a single point.

    Tries WFS point query → WFS bbox fallback → WMS GetFeatureInfo fallback.
    If no layer specified, tries all candidate layers.
    """
    result: dict[str, Any] = {
        "lat": lat,
        "lon": lon,
        "cadastral_code": None,
        "area_m2": None,
        "purpose": None,
        "properties": {},
        "source": None,
        "layer": None,
    }

    layers_to_try = [layer] if layer else CANDIDATE_LAYERS

    for lyr in layers_to_try:
        # Try 1: WFS point query
        features = wfs_point_query(lat, lon, lyr)
        if features:
            props = features[0].get("properties", {})
            result["properties"] = props
            result["source"] = "wfs_point"
            result["layer"] = lyr
            _extract_cadastral(result, props)
            return result

        # Try 2: WFS bbox query
        features = wfs_bbox_query(lat, lon, lyr)
        if features:
            props = features[0].get("properties", {})
            result["properties"] = props
            result["source"] = "wfs_bbox"
            result["layer"] = lyr
            _extract_cadastral(result, props)
            return result

        # Try 3: WMS GetFeatureInfo
        props = wms_getfeatureinfo(lat, lon, lyr)
        if props:
            result["properties"] = props
            result["source"] = "wms_featureinfo"
            result["layer"] = lyr
            _extract_cadastral(result, props)
            return result

    result["source"] = "none"
    return result


def _extract_cadastral(result: dict[str, Any], props: dict[str, Any]) -> None:
    """Extract cadastral code, area, purpose from feature properties."""
    # Try common field names for cadastral code
    for key in ["CADCODE", "cadcode", "CadCode", "cadastral_code", "UNIQ_CODE",
                 "uniq_code", "code", "CODE", "cad_code", "CAD_CODE", "ID", "id"]:
        if key in props and props[key]:
            result["cadastral_code"] = str(props[key])
            break

    # Try common field names for area
    for key in ["AREA", "area", "Area", "SHAPE_AREA", "shape_area", "sq_m",
                 "AREA_M2", "area_m2", "SQUARE"]:
        if key in props and props[key]:
            try:
                result["area_m2"] = float(props[key])
            except (ValueError, TypeError):
                pass
            break

    # Try common field names for purpose/zoning
    for key in ["PURPOSE", "purpose", "Purpose", "LAND_USE", "land_use", "LandUse",
                 "ZONING", "zoning", "TYPE", "type", "CATEGORY", "category"]:
        if key in props and props[key]:
            result["purpose"] = str(props[key])
            break


def batch_from_json(path: str) -> list[dict[str, Any]]:
    """Process Place.ge JSON listings — extract lat/lon and lookup each."""
    with open(path) as f:
        listings = json.load(f)

    if isinstance(listings, dict):
        listings = listings.get("listings", listings.get("data", [listings]))

    results = []
    for i, item in enumerate(listings):
        lat = item.get("lat") or item.get("latitude") or item.get("y")
        lon = item.get("lon") or item.get("lng") or item.get("longitude") or item.get("x")

        if not lat or not lon:
            # Try nested location
            loc = item.get("location", {})
            lat = loc.get("lat") or loc.get("latitude")
            lon = loc.get("lon") or loc.get("lng") or loc.get("longitude")

        if not lat or not lon:
            print(f"  [{i+1}] SKIP — no coordinates", file=sys.stderr)
            continue

        lat, lon = float(lat), float(lon)
        print(f"  [{i+1}/{len(listings)}] Looking up ({lat:.6f}, {lon:.6f})...", file=sys.stderr)

        result = lookup_point(lat, lon)
        result["listing_id"] = item.get("id") or item.get("ID") or i
        result["listing_price"] = item.get("price") or item.get("Price")
        results.append(result)

        time.sleep(REQUEST_DELAY)

    return results


def batch_from_csv(path: str) -> list[dict[str, Any]]:
    """Process CSV with lat,lon per line."""
    results = []
    with open(path) as f:
        for i, line in enumerate(f):
            line = line.strip()
            if not line or line.startswith("#") or line.startswith("lat"):
                continue
            parts = line.split(",")
            if len(parts) < 2:
                continue
            lat, lon = float(parts[0]), float(parts[1])
            print(f"  [{i+1}] Looking up ({lat:.6f}, {lon:.6f})...", file=sys.stderr)
            result = lookup_point(lat, lon)
            results.append(result)
            time.sleep(REQUEST_DELAY)
    return results


def main() -> None:
    parser = argparse.ArgumentParser(description="WFS cadastre lookup by coordinates")
    parser.add_argument("lat", nargs="?", type=float, help="Latitude")
    parser.add_argument("lon", nargs="?", type=float, help="Longitude")
    parser.add_argument("--layer", help="Specific WFS layer name")
    parser.add_argument("--batch", help="Batch from Place.ge JSON file")
    parser.add_argument("--batch-csv", help="Batch from CSV (lat,lon per line)")
    parser.add_argument("--layers", action="store_true", help="Discover available layers")
    parser.add_argument("--json", action="store_true", help="Output as JSON")
    parser.add_argument("--output", "-o", help="Write results to file")
    args = parser.parse_args()

    if args.layers:
        print("Discovering WFS layers...", file=sys.stderr)
        layers = discover_layers()
        if args.json:
            print(json.dumps(layers, indent=2, ensure_ascii=False))
        else:
            print(f"Found {len(layers)} layers:")
            for lyr in layers:
                print(f"  {lyr['name']:40s} {lyr.get('title', '')}")
        return

    if args.batch:
        print(f"Batch processing {args.batch}...", file=sys.stderr)
        results = batch_from_json(args.batch)
    elif args.batch_csv:
        print(f"Batch processing {args.batch_csv}...", file=sys.stderr)
        results = batch_from_csv(args.batch_csv)
    elif args.lat is not None and args.lon is not None:
        results = [lookup_point(args.lat, args.lon, args.layer)]
    else:
        parser.print_help()
        sys.exit(1)

    # Output
    output_text = ""
    if args.json or args.output:
        output_text = json.dumps(results, indent=2, ensure_ascii=False)
    else:
        for r in results:
            lid = r.get("listing_id", "")
            prefix = f"[{lid}] " if lid else ""
            cad = r.get("cadastral_code") or "NOT FOUND"
            area = f"{r['area_m2']:.0f} m²" if r.get("area_m2") else "?"
            purpose = r.get("purpose") or "?"
            src = r.get("source", "?")
            lyr = r.get("layer") or "?"
            output_text += f"{prefix}({r['lat']:.6f}, {r['lon']:.6f}) → {cad}  area={area}  purpose={purpose}  [via {src}, layer={lyr}]\n"

    if args.output:
        Path(args.output).write_text(output_text)
        print(f"Written to {args.output}", file=sys.stderr)
    else:
        print(output_text, end="")

    # Summary
    found = sum(1 for r in results if r.get("cadastral_code"))
    print(f"\nSummary: {found}/{len(results)} points resolved to cadastral codes.", file=sys.stderr)


if __name__ == "__main__":
    main()
