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
ZONES: list[tuple[str, float, float, float, float]] = [
    # Poti city + surroundings
    ("Poti",          41.64, 42.12, 41.72, 42.18),
    # Maltakva / Grigoleti north
    ("Maltakva",      41.68, 42.02, 41.74, 42.12),
    # Grigoleti (resort village)
    ("Grigoleti",     41.68, 41.94, 41.74, 42.02),
    # Ureki (magnetic sand beach)
    ("Ureki",         41.72, 41.93, 41.78, 41.98),
    # Shekvetili (new resort area)
    ("Shekvetili",    41.76, 41.90, 41.82, 41.95),
    # Kobuleti south
    ("Kobuleti-S",    41.78, 41.80, 41.84, 41.88),
    # Kobuleti center + north
    ("Kobuleti-N",    41.76, 41.76, 41.82, 41.82),
    # Tsikhisdziri / Bobokvati
    ("Tsikhisdziri",  41.72, 41.72, 41.78, 41.78),
]


def count_parcels(
    bbox: tuple[float, float, float, float],
) -> dict[str, Any]:
    """Count parcels in bbox using returnCountOnly."""
    xmin, ymin, xmax, ymax = bbox
    params = urllib.parse.urlencode({
        "where": "1=1",
        "geometry": json.dumps({
            "xmin": xmin, "ymin": ymin,
            "xmax": xmax, "ymax": ymax,
            "spatialReference": {"wkid": 4326},
        }),
        "geometryType": "esriGeometryEnvelope",
        "inSR": "4326",
        "spatialRel": "esriSpatialRelIntersects",
        "returnCountOnly": "true",
        "f": "json",
    })
    url = f"{ARCGIS_URL}?{params}"
    req = urllib.request.Request(url, headers={"User-Agent": "plot-bot/1.0"})

    try:
        with urllib.request.urlopen(req, timeout=20) as resp:
            data = json.loads(resp.read().decode("utf-8"))
            return {"count": data.get("count", 0), "error": None}
    except Exception as e:
        return {"count": 0, "error": str(e)}


def get_area_stats(
    bbox: tuple[float, float, float, float],
    sample_size: int = 50,
) -> dict[str, Any]:
    """Get area distribution from a sample of parcels in bbox."""
    xmin, ymin, xmax, ymax = bbox
    params = urllib.parse.urlencode({
        "where": "1=1",
        "geometry": json.dumps({
            "xmin": xmin, "ymin": ymin,
            "xmax": xmax, "ymax": ymax,
            "spatialReference": {"wkid": 4326},
        }),
        "geometryType": "esriGeometryEnvelope",
        "inSR": "4326",
        "spatialRel": "esriSpatialRelIntersects",
        "outFields": "SHAPE.AREA,UNIQ_CODE",
        "returnGeometry": "false",
        "resultRecordCount": str(sample_size),
        "f": "json",
    })
    url = f"{ARCGIS_URL}?{params}"
    req = urllib.request.Request(url, headers={"User-Agent": "plot-bot/1.0"})

    try:
        with urllib.request.urlopen(req, timeout=20) as resp:
            data = json.loads(resp.read().decode("utf-8"))
            features = data.get("features", [])
            areas = []
            for f in features:
                a = f.get("attributes", {}).get("SHAPE.AREA")
                if a:
                    try:
                        areas.append(float(a))
                    except (ValueError, TypeError):
                        pass

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
    except Exception:
        return {}


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
    for name, xmin, ymin, xmax, ymax in ZONES:
        print(f"  {name:15s} ...", end="", file=sys.stderr)
        bbox = (xmin, ymin, xmax, ymax)
        data = count_parcels(bbox)
        count = data["count"]
        total += count

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

        if args.detailed and count > 0 and not data["error"]:
            time.sleep(0.5)
            stats = get_area_stats(bbox)
            if stats:
                zone_data["area_stats"] = stats
                pct = stats.get("pct_over_3000m2", 0)
                print(f"{'':17s} median={stats['median_m2']:.0f}m², "
                      f"≥3000m²={pct}% of sample", file=sys.stderr)

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

    # Estimate relevant parcels (≥500m² based on sample ratios)
    relevant_zones = [z for z in results["zones"] if z.get("area_stats")]
    if relevant_zones:
        avg_pct_large = sum(
            z["area_stats"]["pct_over_3000m2"] for z in relevant_zones
        ) / len(relevant_zones)
        estimated_relevant = int(total * avg_pct_large / 100)
        results["estimated_relevant_3000m2"] = estimated_relevant
        print(f"EST. ≥3000m²: {estimated_relevant:>5,} parcels (~{avg_pct_large:.0f}% of universe)", file=sys.stderr)

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
