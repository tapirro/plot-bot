#!/usr/bin/env python3
"""Universe Census — count all cadastral parcels in target zone.

Queries ArcGIS MapServer Layer 14 to enumerate total parcels in the
Poti–Kobuleti coastal corridor. Outputs zone-by-zone counts and totals.

This defines the "universe" — 100% coverage means every parcel is in our DB.

Usage:
    python3 tools/scripts/universe_census.py
    python3 tools/scripts/universe_census.py --json
    python3 tools/scripts/universe_census.py --output work/data/universe_census.json
    python3 tools/scripts/universe_census.py --detailed  # per-zone breakdown
"""
from __future__ import annotations

import argparse
import json
import sys
import time
import urllib.parse
import urllib.request
from typing import Any

ARCGIS_URL = (
    "http://gisappsn.reestri.gov.ge/ArcGIS/rest/services"
    "/CadRepGeo/MapServer/14/query"
)

# Target zones: Poti → Kobuleti coastal corridor
# Each zone: (name, xmin/lon_min, ymin/lat_min, xmax/lon_max, ymax/lat_max)
# Bounding boxes: ~3km from coast inland
# Format: (name, lon_min, lat_min, lon_max, lat_max) — WGS84
# Verified against arcgis_spatial.py working presets
ZONES: list[tuple[str, float, float, float, float]] = [
    # Guria coast (north to south)
    ("Poti",          41.62, 42.10, 41.72, 42.20),   # Layer 14 may not cover
    ("Grigoleti",     41.68, 41.73, 41.73, 41.78),   # from arcgis preset
    ("Ureki",         41.72, 41.75, 41.77, 41.78),   # from arcgis preset (verified: 1947)
    # Transition zone
    ("Shekvetili",    41.75, 41.78, 41.80, 41.82),
    # Adjara coast (Kobuleti corridor)
    ("Kobuleti-S",    41.78, 41.80, 41.84, 41.88),   # verified: 6340
    ("Kobuleti-N",    41.76, 41.76, 41.82, 41.82),   # verified: 8474
    ("Tsikhisdziri",  41.72, 41.72, 41.78, 41.78),   # verified: 7206
]


PAGE_LIMIT = 500  # ArcGIS server hard spatial limit


def _query_bbox(geom_str: str) -> list[dict[str, Any]]:
    """Single ArcGIS query — returns up to 500 features."""
    params = urllib.parse.urlencode({
        "where": "1=1",
        "geometry": geom_str,
        "geometryType": "esriGeometryEnvelope",
        "inSR": "4326",
        "spatialRel": "esriSpatialRelIntersects",
        "outFields": "OBJECTID,SHAPE.AREA",
        "returnGeometry": "false",
        "f": "json",
    })
    url = f"{ARCGIS_URL}?{params}"
    req = urllib.request.Request(url, headers={"User-Agent": "plot-bot/1.0"})

    with urllib.request.urlopen(req, timeout=30) as resp:
        data = json.loads(resp.read().decode("utf-8"))
    return data.get("features", [])


def count_parcels(
    bbox: tuple[float, float, float, float],
    depth: int = 0,
    max_depth: int = 6,
) -> dict[str, Any]:
    """Count parcels using quadtree subdivision.

    ArcGIS server has a hard 500-feature spatial limit with no real pagination.
    If a bbox returns exactly 500, subdivide into 4 quadrants and recurse.
    Deduplicates by OBJECTID.
    """
    xmin, ymin, xmax, ymax = bbox
    geom_str = f"{xmin},{ymin},{xmax},{ymax}"

    seen_oids: set[int] = set()
    areas: list[float] = []

    try:
        features = _query_bbox(geom_str)

        if len(features) < PAGE_LIMIT or depth >= max_depth:
            # This tile fits in one page — count directly
            for f in features:
                attrs = f.get("attributes", {})
                oid = attrs.get("OBJECTID", 0)
                seen_oids.add(oid)
                a = attrs.get("SHAPE.AREA")
                if a:
                    try:
                        areas.append(float(a))
                    except (ValueError, TypeError):
                        pass
            return {"count": len(seen_oids), "areas": areas, "oids": seen_oids, "error": None}

        # Too many features — subdivide into 4 quadrants
        mx = (xmin + xmax) / 2
        my = (ymin + ymax) / 2
        quads = [
            (xmin, ymin, mx, my),    # SW
            (mx, ymin, xmax, my),    # SE
            (xmin, my, mx, ymax),    # NW
            (mx, my, xmax, ymax),    # NE
        ]

        for q in quads:
            time.sleep(0.2)
            sub = count_parcels(q, depth + 1, max_depth)
            # Deduplicate across quadrants (parcels on boundaries)
            for oid in sub.get("oids", set()):
                seen_oids.add(oid)
            areas.extend(sub.get("areas", []))
            if sub.get("error"):
                return {"count": len(seen_oids), "areas": areas, "oids": seen_oids, "error": sub["error"]}

        return {"count": len(seen_oids), "areas": areas, "oids": seen_oids, "error": None}

    except Exception as e:
        return {"count": len(seen_oids), "areas": areas, "oids": seen_oids, "error": str(e)}


def compute_area_stats(areas: list[float]) -> dict[str, Any]:
    """Compute area distribution stats from collected areas."""
    if not areas:
        return {}

    areas.sort()
    n = len(areas)
    return {
        "sample_size": n,
        "min_m2": round(min(areas), 1),
        "max_m2": round(max(areas), 1),
        "median_m2": round(areas[n // 2], 1),
        "mean_m2": round(sum(areas) / n, 1),
        "over_500m2": sum(1 for a in areas if a >= 500),
        "over_1000m2": sum(1 for a in areas if a >= 1000),
        "over_3000m2": sum(1 for a in areas if a >= 3000),
        "pct_over_3000m2": round(sum(1 for a in areas if a >= 3000) / n * 100, 1),
    }


def count_listings_in_db(db_path: str) -> dict[str, int]:
    """Count listings and scored parcels in land.db."""
    try:
        import sqlite3
        conn = sqlite3.connect(db_path)
        c = conn.cursor()

        counts: dict[str, int] = {}
        for table in ("listings", "parcels", "scores"):
            try:
                c.execute(f"SELECT COUNT(*) FROM {table}")
                counts[table] = c.fetchone()[0]
            except Exception:
                counts[table] = 0

        conn.close()
        return counts
    except Exception:
        return {"listings": 0, "parcels": 0, "scores": 0}


def main() -> None:
    parser = argparse.ArgumentParser(description="Count all parcels in target zone")
    parser.add_argument("--json", action="store_true", help="JSON output")
    parser.add_argument("--output", "-o", help="Write to file")
    parser.add_argument("--detailed", action="store_true", help="Include area stats per zone")
    parser.add_argument("--db", default="work/data/land.db", help="Path to land.db")
    args = parser.parse_args()

    print("Universe Census: Poti → Kobuleti corridor", file=sys.stderr)
    print("=" * 50, file=sys.stderr)

    results = {
        "corridor": "Poti → Kobuleti",
        "zones": [],
        "total_parcels": 0,
        "db_coverage": {},
        "timestamp": __import__("datetime").datetime.now().isoformat(),
    }

    total = 0
    all_areas: list[float] = []
    for name, xmin, ymin, xmax, ymax in ZONES:
        print(f"  {name:15s} ...", end="", flush=True, file=sys.stderr)
        bbox = (xmin, ymin, xmax, ymax)
        data = count_parcels(bbox)
        count = data["count"]
        total += count
        areas = data.get("areas", [])
        all_areas.extend(areas)

        zone_data: dict[str, Any] = {
            "name": name,
            "bbox": [xmin, ymin, xmax, ymax],
            "parcels": count,
        }

        if data["error"]:
            print(f" ERROR: {data['error']}", file=sys.stderr)
            zone_data["error"] = data["error"]
        else:
            print(f" {count:>6,} parcels", file=sys.stderr)

        if areas:
            stats = compute_area_stats(areas)
            zone_data["area_stats"] = stats
            if args.detailed:
                pct = stats.get("pct_over_3000m2", 0)
                print(f"{'':17s} median={stats['median_m2']:.0f}m², "
                      f"≥3000m²={pct}%", file=sys.stderr)

        results["zones"].append(zone_data)
        time.sleep(0.3)

    results["total_parcels"] = total

    # Check DB coverage
    from pathlib import Path
    db_path = Path(args.db)
    if db_path.exists():
        results["db_coverage"] = count_listings_in_db(str(db_path))

    # Summary
    db = results["db_coverage"]
    db_parcels = db.get("parcels", 0)
    db_listings = db.get("listings", 0)
    db_scores = db.get("scores", 0)

    print(f"\n{'=' * 50}", file=sys.stderr)
    print(f"UNIVERSE:  {total:>8,} cadastral parcels", file=sys.stderr)
    print(f"IN DB:     {db_parcels:>8,} parcels ({db_parcels/total*100:.1f}%)" if total else "", file=sys.stderr)
    print(f"LISTINGS:  {db_listings:>8,}", file=sys.stderr)
    print(f"SCORED:    {db_scores:>8,} ({db_scores/total*100:.1f}%)" if total else "", file=sys.stderr)

    # Compute relevant parcels from actual data
    if all_areas:
        total_stats = compute_area_stats(all_areas)
        results["area_stats_total"] = total_stats
        over_3k = total_stats.get("over_3000m2", 0)
        over_500 = total_stats.get("over_500m2", 0)
        results["relevant_3000m2"] = over_3k
        results["relevant_500m2"] = over_500
        print(f"≥500m²:    {over_500:>8,} parcels ({over_500/total*100:.1f}%)" if total else "", file=sys.stderr)
        print(f"≥3000m²:   {over_3k:>8,} parcels ({over_3k/total*100:.1f}%) ← target for scoring", file=sys.stderr)

    # Output
    if args.json or args.output:
        text = json.dumps(results, indent=2, ensure_ascii=False)
        if args.output:
            Path(args.output).write_text(text)
            print(f"\nWritten to {args.output}", file=sys.stderr)
        else:
            print(text)
    elif not args.json:
        # Human-readable table
        print(f"\n{'Zone':<17} {'Parcels':>8}")
        print("-" * 27)
        for z in results["zones"]:
            print(f"{z['name']:<17} {z['parcels']:>8,}")
        print("-" * 27)
        print(f"{'TOTAL':<17} {total:>8,}")


if __name__ == "__main__":
    main()
