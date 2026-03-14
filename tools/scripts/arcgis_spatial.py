#!/usr/bin/env python3
"""ArcGIS Spatial Query — Georgian Cadastral Parcels.

Queries CadRepGeo MapServer Layer 14 (Parcels) by bounding box.
Returns all parcels within a geographic area with their UNIQ_CODE,
area, and geometry. Useful for land prospecting in target zones.

Usage:
    python3 arcgis_spatial.py --preset ureki
    python3 arcgis_spatial.py --preset ureki --output /tmp/ureki_parcels.json
    python3 arcgis_spatial.py --bbox 41.72,41.75,41.77,41.78
    python3 arcgis_spatial.py --bbox 41.72,41.75,41.77,41.78 --no-geometry
    python3 arcgis_spatial.py --preset ureki --dry-run

API notes:
    - HTTP only (not HTTPS!) — gisappsn.reestri.gov.ge
    - Native SRID: EPSG:32638 (UTM zone 38N)
    - Input/output converted to EPSG:4326 (WGS84) via inSR/outSR
    - Server returns max 500 features per request
    - Bbox format: xmin,ymin,xmax,ymax (lon_min,lat_min,lon_max,lat_max)
    - Geometry rings: [[lon, lat], ...] in WGS84
"""

import argparse
import json
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from typing import Any

ARCGIS_LAYER_URL = (
    "http://gisappsn.reestri.gov.ge/ArcGIS/rest/services"
    "/CadRepGeo/MapServer/14/query"
)

SERVER_PAGE_LIMIT = 500

# Predefined bounding boxes: (xmin/lon_min, ymin/lat_min, xmax/lon_max, ymax/lat_max)
PRESETS: dict[str, tuple[float, float, float, float]] = {
    "ureki":    (41.72, 41.75, 41.77, 41.78),
    "grigoleti": (41.68, 41.73, 41.73, 41.76),
    "kobuleti": (41.75, 41.78, 41.79, 41.83),
}


def log(msg: str) -> None:
    """Log to stderr."""
    print(json.dumps({"agent": "arcgis_spatial", "msg": msg}), file=sys.stderr)


def query_page(
    bbox: tuple[float, float, float, float],
    *,
    return_geometry: bool = True,
    min_oid: int = 0,
) -> dict[str, Any]:
    """Query one page of parcels within bbox.

    Args:
        bbox: (xmin, ymin, xmax, ymax) in WGS84 lon/lat.
        return_geometry: Include polygon geometry in response.
        min_oid: Fetch only features with OBJECTID > min_oid (for pagination).

    Returns:
        Raw ArcGIS JSON response.
    """
    where = f"OBJECTID>{min_oid}" if min_oid > 0 else "1=1"
    params = {
        "geometry": f"{bbox[0]},{bbox[1]},{bbox[2]},{bbox[3]}",
        "geometryType": "esriGeometryEnvelope",
        "inSR": "4326",
        "spatialRel": "esriSpatialRelIntersects",
        "outFields": "OBJECTID,UNIQ_CODE,SHAPE.AREA,SHAPE.LEN,DANISHNULEBA,TARIRI",
        "returnGeometry": "true" if return_geometry else "false",
        "outSR": "4326",
        "orderByFields": "OBJECTID ASC",
        "where": where,
        "f": "json",
    }
    url = ARCGIS_LAYER_URL + "?" + urllib.parse.urlencode(params)

    req = urllib.request.Request(url)
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.loads(resp.read().decode())


def query_all(
    bbox: tuple[float, float, float, float],
    *,
    return_geometry: bool = True,
    delay: float = 0.5,
) -> list[dict[str, Any]]:
    """Query all parcels within bbox, paginating if needed.

    The ArcGIS server returns max 500 features per request.
    Pagination uses OBJECTID > last_seen_oid.

    Args:
        bbox: (xmin, ymin, xmax, ymax) in WGS84 lon/lat.
        return_geometry: Include polygon geometry in response.
        delay: Seconds between paginated requests.

    Returns:
        List of feature dicts with 'attributes' and optionally 'geometry'.
    """
    all_features: list[dict[str, Any]] = []
    min_oid = 0
    page = 0

    while True:
        page += 1
        log(f"page={page} min_oid={min_oid} accumulated={len(all_features)}")

        try:
            data = query_page(bbox, return_geometry=return_geometry, min_oid=min_oid)
        except urllib.error.URLError as e:
            log(f"request failed: {e}")
            break

        if "error" in data:
            log(f"ArcGIS error: {data['error']}")
            break

        features = data.get("features", [])
        if not features:
            break

        all_features.extend(features)

        if len(features) < SERVER_PAGE_LIMIT:
            break

        # Next page: use max OBJECTID from current batch
        max_oid = max(f["attributes"]["OBJECTID"] for f in features)
        min_oid = max_oid

        if delay > 0:
            time.sleep(delay)

    return all_features


def extract_parcel(feature: dict[str, Any]) -> dict[str, Any]:
    """Extract clean parcel record from raw ArcGIS feature."""
    attrs = feature.get("attributes", {})
    record: dict[str, Any] = {
        "uniq_code": attrs.get("UNIQ_CODE", ""),
        "area_sqm": attrs.get("SHAPE.AREA"),
        "perimeter_m": attrs.get("SHAPE.LEN"),
        "purpose": attrs.get("DANISHNULEBA"),
        "rate": attrs.get("TARIRI"),
        "object_id": attrs.get("OBJECTID"),
    }
    geom = feature.get("geometry")
    if geom and "rings" in geom:
        record["geometry"] = geom["rings"]
    return record


def subdivide_bbox(
    bbox: tuple[float, float, float, float],
    nx: int = 2,
    ny: int = 2,
) -> list[tuple[float, float, float, float]]:
    """Split a bounding box into nx * ny sub-boxes.

    Useful when a single bbox returns exactly SERVER_PAGE_LIMIT features,
    suggesting truncation.
    """
    xmin, ymin, xmax, ymax = bbox
    dx = (xmax - xmin) / nx
    dy = (ymax - ymin) / ny
    subs = []
    for ix in range(nx):
        for iy in range(ny):
            subs.append((
                xmin + ix * dx,
                ymin + iy * dy,
                xmin + (ix + 1) * dx,
                ymin + (iy + 1) * dy,
            ))
    return subs


def query_with_subdivision(
    bbox: tuple[float, float, float, float],
    *,
    return_geometry: bool = True,
    delay: float = 0.5,
    max_depth: int = 3,
) -> list[dict[str, Any]]:
    """Query parcels, subdividing bbox if server limit is hit.

    If a query returns exactly SERVER_PAGE_LIMIT features (possible
    truncation), the bbox is split into 4 quadrants and each is queried
    separately. Deduplication by OBJECTID ensures no duplicates.

    Args:
        bbox: (xmin, ymin, xmax, ymax) in WGS84 lon/lat.
        return_geometry: Include polygon geometry.
        delay: Seconds between requests.
        max_depth: Maximum recursion depth for subdivision.

    Returns:
        Deduplicated list of features.
    """
    features = query_all(bbox, return_geometry=return_geometry, delay=delay)

    if len(features) >= SERVER_PAGE_LIMIT and max_depth > 0:
        log(f"hit limit={len(features)}, subdividing bbox (depth={max_depth})")
        seen_oids: set[int] = set()
        all_features: list[dict[str, Any]] = []

        for sub_bbox in subdivide_bbox(bbox):
            sub_features = query_with_subdivision(
                sub_bbox,
                return_geometry=return_geometry,
                delay=delay,
                max_depth=max_depth - 1,
            )
            for f in sub_features:
                oid = f["attributes"]["OBJECTID"]
                if oid not in seen_oids:
                    seen_oids.add(oid)
                    all_features.append(f)

        return all_features

    return features


def main() -> None:
    parser = argparse.ArgumentParser(
        description="ArcGIS Spatial Query — Georgian Cadastral Parcels"
    )
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument(
        "--preset",
        choices=list(PRESETS.keys()),
        help="Predefined area bounding box",
    )
    group.add_argument(
        "--bbox",
        help="Custom bbox: xmin,ymin,xmax,ymax (lon_min,lat_min,lon_max,lat_max)",
    )
    parser.add_argument(
        "--output", "-o",
        help="Output file path (default: stdout)",
    )
    parser.add_argument(
        "--no-geometry",
        action="store_true",
        help="Omit polygon geometry (smaller output)",
    )
    parser.add_argument(
        "--delay",
        type=float,
        default=0.5,
        help="Delay between paginated requests in seconds (default: 0.5)",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Print query parameters without executing",
    )
    args = parser.parse_args()

    if args.preset:
        bbox = PRESETS[args.preset]
        area_name = args.preset
    else:
        try:
            parts = [float(x) for x in args.bbox.split(",")]
            if len(parts) != 4:
                raise ValueError
            bbox = (parts[0], parts[1], parts[2], parts[3])
            area_name = "custom"
        except (ValueError, AttributeError):
            print("Error: --bbox must be 4 comma-separated floats: xmin,ymin,xmax,ymax",
                  file=sys.stderr)
            sys.exit(1)

    if args.dry_run:
        print(json.dumps({
            "action": "arcgis_spatial_query",
            "area": area_name,
            "bbox": list(bbox),
            "return_geometry": not args.no_geometry,
            "url_base": ARCGIS_LAYER_URL,
        }, indent=2))
        return

    log(f"querying area={area_name} bbox={bbox}")
    return_geometry = not args.no_geometry

    features = query_with_subdivision(
        bbox,
        return_geometry=return_geometry,
        delay=args.delay,
    )

    parcels = [extract_parcel(f) for f in features]
    # Remove duplicates by uniq_code (parcels on sub-box boundaries)
    seen: set[str] = set()
    unique_parcels: list[dict[str, Any]] = []
    for p in parcels:
        code = p["uniq_code"]
        if code and code not in seen:
            seen.add(code)
            unique_parcels.append(p)
        elif not code:
            unique_parcels.append(p)

    output = {
        "area": area_name,
        "bbox": list(bbox),
        "total_parcels": len(unique_parcels),
        "parcels": unique_parcels,
    }

    result = json.dumps(output, indent=2, ensure_ascii=False)

    if args.output:
        with open(args.output, "w") as f:
            f.write(result)
        log(f"wrote {len(unique_parcels)} parcels to {args.output}")
    else:
        print(result)

    log(f"done: {len(unique_parcels)} parcels found")


if __name__ == "__main__":
    main()
