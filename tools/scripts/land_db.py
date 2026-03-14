#!/usr/bin/env python3
"""Land Database — SQLite storage + ETL for listings and parcels.

Schema covers:
  - listings: Place.ge (and future SS.ge, Myhome.ge) land listings
  - parcels: ArcGIS cadastral parcels
  - scores: scoring model results (scoring_model.md)
  - listing_parcel: many-to-many link between listings and parcels

ETL loads existing JSON data from work/data/.

Usage:
    python3 tools/scripts/land_db.py init                    # create DB
    python3 tools/scripts/land_db.py load-listings FILE      # load Place.ge JSON
    python3 tools/scripts/land_db.py load-parcels FILE       # load ArcGIS JSON
    python3 tools/scripts/land_db.py load-all                # load all JSONs from work/data/
    python3 tools/scripts/land_db.py stats                   # show DB statistics
    python3 tools/scripts/land_db.py query --min-area 3000 --max-price 40  # filter listings
    python3 tools/scripts/land_db.py export [--format json|csv]  # export filtered data
"""

import argparse
import csv
import io
import json
import logging
import math
import os
import sqlite3
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Optional

PROJECT_ROOT = Path(__file__).resolve().parent.parent.parent
DB_PATH = PROJECT_ROOT / "work" / "data" / "land.db"
DATA_DIR = PROJECT_ROOT / "work" / "data"

logging.basicConfig(
    format='{"ts":"%(asctime)s","level":"%(levelname)s","msg":"%(message)s"}',
    level=logging.INFO,
    datefmt="%Y-%m-%dT%H:%M:%S",
)
log = logging.getLogger("land_db")

SCHEMA_VERSION = 2

MIGRATIONS = {
    2: """
-- v2: Add epistemology fields (confidence, verified_at, stale)
ALTER TABLE listings ADD COLUMN confidence REAL DEFAULT 0.45;
ALTER TABLE listings ADD COLUMN verified_at TEXT;
ALTER TABLE listings ADD COLUMN stale INTEGER DEFAULT 0;
ALTER TABLE parcels ADD COLUMN confidence REAL DEFAULT 0.85;
ALTER TABLE parcels ADD COLUMN verified_at TEXT;
ALTER TABLE parcels ADD COLUMN stale INTEGER DEFAULT 0;
ALTER TABLE scores ADD COLUMN confidence REAL;
-- Backfill scores.confidence from mode
UPDATE scores SET confidence = CASE mode
    WHEN 'remote' THEN 0.35
    WHEN 'enriched' THEN 0.60
    WHEN 'verified' THEN 0.85
    ELSE 0.30
END WHERE confidence IS NULL;
""",
}

SCHEMA_SQL = """
-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Land listings from marketplaces (Place.ge, SS.ge, Myhome.ge)
CREATE TABLE IF NOT EXISTS listings (
    id              INTEGER NOT NULL,      -- source listing ID
    source          TEXT    NOT NULL,      -- 'place.ge', 'ss.ge', 'myhome.ge'
    status          TEXT,                  -- 'FOR SALE', 'SOLD', etc.
    area_sqm        REAL,
    location        TEXT,                  -- raw location string
    location_city   TEXT,                  -- normalized city name
    location_zone   TEXT,                  -- scoring zone (ureki, grigoleti, batumi, etc.)
    price_total_usd REAL,
    price_per_sqm   REAL,
    price_text      TEXT,                  -- original price text
    is_urgent       INTEGER DEFAULT 0,
    photo_count     INTEGER DEFAULT 0,
    contact         TEXT,
    url             TEXT,
    scraped_at      TEXT,                  -- ISO timestamp
    confidence      REAL    DEFAULT 0.45,  -- epistemology: L4 single source = 0.45
    verified_at     TEXT,                  -- last verification date (NULL = never)
    stale           INTEGER DEFAULT 0,     -- auto-set when scraped_at > 7-day SLA
    created_at      TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at      TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    PRIMARY KEY (source, id)
);

-- ArcGIS cadastral parcels
CREATE TABLE IF NOT EXISTS parcels (
    uniq_code    TEXT    PRIMARY KEY,  -- 9-digit cadastral code
    area_sqm     REAL,
    perimeter_m  REAL,
    compactness  REAL,                -- 4*pi*area/perimeter^2 (precomputed)
    purpose      INTEGER,             -- land use code
    rate_date    TEXT,                 -- registration date
    object_id    INTEGER,
    source_area  TEXT,                 -- 'ureki', 'grigoleti', etc.
    bbox_lat_min REAL,
    bbox_lon_min REAL,
    bbox_lat_max REAL,
    bbox_lon_max REAL,
    confidence   REAL    DEFAULT 0.85,  -- epistemology: L2 official registry = 0.85
    verified_at  TEXT,                  -- last verification date
    stale        INTEGER DEFAULT 0,     -- auto-set when created_at > 30-day SLA
    created_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

-- Scoring results (from scoring_model.md)
CREATE TABLE IF NOT EXISTS scores (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    listing_source  TEXT,
    listing_id      INTEGER,
    parcel_code     TEXT,
    mode            TEXT    NOT NULL,  -- 'remote', 'enriched', 'verified'
    -- Filter results (pass/fail)
    f1_price        INTEGER,  -- 1=pass, 0=fail
    f2_area         INTEGER,
    f3_sea_dist     INTEGER,
    f4_legal        INTEGER,
    f5_purpose      INTEGER,
    f6_visual       INTEGER,
    f7_access       INTEGER,
    filters_passed  INTEGER,  -- 1=all passed, 0=at least one failed
    -- Category scores (0-max)
    s1_price_potential  REAL,  -- max 30
    s2_location         REAL,  -- max 25
    s3_parcel           REAL,  -- max 20
    s4_infrastructure   REAL,  -- max 15
    s5_cluster          REAL,  -- max 10
    total_score         REAL,  -- 0-100
    score_class         TEXT,  -- A/B/C/D/F
    confidence          REAL,  -- epistemology: derived from mode (remote=0.35, enriched=0.60, verified=0.85)
    -- Metadata
    scored_at       TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    notes           TEXT,
    FOREIGN KEY (listing_source, listing_id) REFERENCES listings(source, id),
    FOREIGN KEY (parcel_code) REFERENCES parcels(uniq_code)
);

-- Many-to-many: link listings to parcels (when matched)
CREATE TABLE IF NOT EXISTS listing_parcel (
    listing_source  TEXT    NOT NULL,
    listing_id      INTEGER NOT NULL,
    parcel_code     TEXT    NOT NULL,
    match_method    TEXT,      -- 'manual', 'cadastral_id', 'spatial'
    confidence      REAL,     -- 0.0-1.0
    created_at      TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    PRIMARY KEY (listing_source, listing_id, parcel_code),
    FOREIGN KEY (parcel_code) REFERENCES parcels(uniq_code)
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_listings_zone ON listings(location_zone);
CREATE INDEX IF NOT EXISTS idx_listings_price ON listings(price_per_sqm);
CREATE INDEX IF NOT EXISTS idx_listings_area ON listings(area_sqm);
CREATE INDEX IF NOT EXISTS idx_parcels_area ON parcels(source_area);
CREATE INDEX IF NOT EXISTS idx_scores_class ON scores(score_class);
CREATE INDEX IF NOT EXISTS idx_scores_total ON scores(total_score);
"""


def normalize_zone(location: str) -> Optional[str]:
    """Map raw location string to a scoring zone."""
    if not location:
        return None
    loc = location.lower()
    zone_map = {
        "ureki": "ureki",
        "უკეთი": "ureki",
        "grigoleti": "grigoleti",
        "გრიგოლეთი": "grigoleti",
        "kobuleti": "kobuleti",
        "ქობულეთი": "kobuleti",
        "batumi": "batumi",
        "ბათუმი": "batumi",
        "ozurgeti": "ozurgeti",
        "ოზურგეთი": "ozurgeti",
        "natanebi": "natanebi",
        "ნატანები": "natanebi",
        "ახალშენი": "batumi",
    }
    for key, zone in zone_map.items():
        if key in loc:
            return zone
    return None


def normalize_city(location: str) -> Optional[str]:
    """Extract city name from location string."""
    if not location:
        return None
    # Take part before comma if present
    city = location.split(",")[0].strip()
    return city if city else None


def compute_compactness(area_sqm: float, perimeter_m: float) -> Optional[float]:
    """Compute shape compactness: 4*pi*area/perimeter^2. Circle=1.0, square≈0.785."""
    if not area_sqm or not perimeter_m or perimeter_m == 0:
        return None
    return (4 * math.pi * area_sqm) / (perimeter_m ** 2)


def _get_db_version(conn: sqlite3.Connection) -> int:
    """Get current schema version from DB, or 0 if not initialized."""
    try:
        row = conn.execute("SELECT value FROM schema_meta WHERE key='version'").fetchone()
        return int(row[0]) if row else 0
    except sqlite3.OperationalError:
        return 0


def _run_migrations(conn: sqlite3.Connection) -> None:
    """Apply pending migrations."""
    current = _get_db_version(conn)
    for ver in sorted(MIGRATIONS.keys()):
        if ver > current:
            log.info("applying migration v%d", ver)
            for stmt in MIGRATIONS[ver].strip().split(";"):
                stmt = stmt.strip()
                if stmt and not stmt.startswith("--"):
                    try:
                        conn.execute(stmt)
                    except sqlite3.OperationalError as e:
                        if "duplicate column" not in str(e).lower():
                            raise
                        log.debug("column already exists, skipping: %s", e)
            conn.execute(
                "INSERT OR REPLACE INTO schema_meta (key, value) VALUES (?, ?)",
                ("version", str(ver)),
            )
            conn.commit()
            log.info("migrated to v%d", ver)


def init_db(db_path: Path) -> sqlite3.Connection:
    """Create database and initialize schema."""
    db_path.parent.mkdir(parents=True, exist_ok=True)
    conn = sqlite3.connect(str(db_path))
    conn.execute("PRAGMA journal_mode=WAL")
    conn.execute("PRAGMA foreign_keys=ON")
    conn.executescript(SCHEMA_SQL)
    conn.execute(
        "INSERT OR REPLACE INTO schema_meta (key, value) VALUES (?, ?)",
        ("version", str(SCHEMA_VERSION)),
    )
    conn.commit()
    # Run any pending migrations for existing DBs
    _run_migrations(conn)
    log.info("database initialized at %s (schema v%d)", db_path, SCHEMA_VERSION)
    return conn


def load_listings(conn: sqlite3.Connection, json_path: Path) -> int:
    """Load Place.ge (or similar) listings JSON into the database.

    Returns number of rows upserted.
    """
    with open(json_path) as f:
        data = json.load(f)

    source = data.get("source", "place.ge")
    scraped_at = data.get("scraped_at")
    listings = data.get("listings", [])

    if not listings:
        log.warning("no listings in %s", json_path)
        return 0

    count = 0
    for item in listings:
        listing_id = item.get("id")
        if listing_id is None:
            continue

        location = item.get("location", "")
        conn.execute(
            """INSERT INTO listings
                (id, source, status, area_sqm, location, location_city,
                 location_zone, price_total_usd, price_per_sqm, price_text,
                 is_urgent, photo_count, contact, url, scraped_at)
               VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
               ON CONFLICT(source, id) DO UPDATE SET
                 status=excluded.status, area_sqm=excluded.area_sqm,
                 price_total_usd=excluded.price_total_usd,
                 price_per_sqm=excluded.price_per_sqm,
                 price_text=excluded.price_text,
                 is_urgent=excluded.is_urgent,
                 photo_count=excluded.photo_count,
                 updated_at=strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
            """,
            (
                listing_id,
                source,
                item.get("status"),
                item.get("area_sqm"),
                location,
                normalize_city(location),
                normalize_zone(location),
                item.get("price_total_usd"),
                item.get("price_per_sqm_usd"),
                item.get("price_text"),
                1 if item.get("is_urgent") else 0,
                item.get("photo_count", 0),
                item.get("contact", "").strip(),
                item.get("url"),
                scraped_at,
            ),
        )
        count += 1

    conn.commit()
    log.info("loaded %d listings from %s (source=%s)", count, json_path.name, source)
    return count


def load_parcels(conn: sqlite3.Connection, json_path: Path) -> int:
    """Load ArcGIS parcels JSON into the database.

    Returns number of rows upserted.
    """
    with open(json_path) as f:
        data = json.load(f)

    source_area = data.get("area", "unknown")
    bbox = data.get("bbox", [None, None, None, None])
    parcels = data.get("parcels", [])

    if not parcels:
        log.warning("no parcels in %s", json_path)
        return 0

    count = 0
    for item in parcels:
        code = item.get("uniq_code")
        if not code:
            continue

        area = item.get("area_sqm")
        perimeter = item.get("perimeter_m")
        compactness = compute_compactness(area, perimeter)

        # Parse rate date
        rate_raw = item.get("rate")
        rate_date = None
        if rate_raw and isinstance(rate_raw, str):
            try:
                dt = datetime.strptime(rate_raw.split(" ")[0], "%m/%d/%Y")
                rate_date = dt.strftime("%Y-%m-%d")
            except (ValueError, IndexError):
                rate_date = rate_raw

        conn.execute(
            """INSERT INTO parcels
                (uniq_code, area_sqm, perimeter_m, compactness, purpose,
                 rate_date, object_id, source_area,
                 bbox_lat_min, bbox_lon_min, bbox_lat_max, bbox_lon_max)
               VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
               ON CONFLICT(uniq_code) DO UPDATE SET
                 area_sqm=excluded.area_sqm,
                 perimeter_m=excluded.perimeter_m,
                 compactness=excluded.compactness,
                 purpose=excluded.purpose,
                 rate_date=excluded.rate_date,
                 updated_at=strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
            """,
            (
                code,
                area,
                perimeter,
                compactness,
                item.get("purpose"),
                rate_date,
                item.get("object_id"),
                source_area,
                bbox[0] if len(bbox) > 0 else None,
                bbox[1] if len(bbox) > 1 else None,
                bbox[2] if len(bbox) > 2 else None,
                bbox[3] if len(bbox) > 3 else None,
            ),
        )
        count += 1

    conn.commit()
    log.info("loaded %d parcels from %s (area=%s)", count, json_path.name, source_area)
    return count


def load_all(conn: sqlite3.Connection) -> dict[str, int]:
    """Load all JSON files from work/data/."""
    results: dict[str, int] = {"listings": 0, "parcels": 0}

    for path in sorted(DATA_DIR.glob("*.json")):
        with open(path) as f:
            try:
                data = json.load(f)
            except json.JSONDecodeError:
                log.warning("skipping invalid JSON: %s", path.name)
                continue

        if "listings" in data:
            results["listings"] += load_listings(conn, path)
        elif "parcels" in data:
            results["parcels"] += load_parcels(conn, path)
        else:
            log.info("skipping unknown format: %s", path.name)

    return results


def show_stats(conn: sqlite3.Connection) -> None:
    """Print database statistics."""
    cur = conn.cursor()

    listing_count = cur.execute("SELECT COUNT(*) FROM listings").fetchone()[0]
    priced_count = cur.execute(
        "SELECT COUNT(*) FROM listings WHERE price_per_sqm IS NOT NULL"
    ).fetchone()[0]
    parcel_count = cur.execute("SELECT COUNT(*) FROM parcels").fetchone()[0]
    score_count = cur.execute("SELECT COUNT(*) FROM scores").fetchone()[0]

    print(f"=== Land Database Stats ===")
    print(f"Listings:  {listing_count} ({priced_count} with price)")
    print(f"Parcels:   {parcel_count}")
    print(f"Scores:    {score_count}")

    # Zone breakdown
    zones = cur.execute(
        """SELECT location_zone, COUNT(*),
                  ROUND(AVG(price_per_sqm), 1),
                  ROUND(MIN(price_per_sqm), 1),
                  ROUND(MAX(price_per_sqm), 1)
           FROM listings
           WHERE price_per_sqm IS NOT NULL
           GROUP BY location_zone ORDER BY COUNT(*) DESC"""
    ).fetchall()

    if zones:
        print(f"\n--- Listings by Zone ---")
        print(f"{'Zone':<15} {'Count':>5} {'Avg $/m²':>10} {'Min':>8} {'Max':>8}")
        for zone, cnt, avg, mn, mx in zones:
            z = zone or "(unknown)"
            print(f"{z:<15} {cnt:>5} {avg or 0:>10.1f} {mn or 0:>8.1f} {mx or 0:>8.1f}")

    # Parcel areas
    areas = cur.execute(
        """SELECT source_area, COUNT(*),
                  ROUND(AVG(area_sqm), 0),
                  ROUND(AVG(compactness), 3)
           FROM parcels GROUP BY source_area"""
    ).fetchall()

    if areas:
        print(f"\n--- Parcels by Area ---")
        print(f"{'Area':<15} {'Count':>7} {'Avg m²':>10} {'Avg compact':>12}")
        for area, cnt, avg_area, avg_comp in areas:
            print(
                f"{area:<15} {cnt:>7} {avg_area or 0:>10.0f} {avg_comp or 0:>12.3f}"
            )


def query_listings(
    conn: sqlite3.Connection,
    min_area: Optional[float] = None,
    max_price: Optional[float] = None,
    zone: Optional[str] = None,
    limit: int = 50,
) -> list[dict[str, Any]]:
    """Query listings with filters. Returns list of dicts."""
    conditions = ["1=1"]
    params: list[Any] = []

    if min_area is not None:
        conditions.append("area_sqm >= ?")
        params.append(min_area)
    if max_price is not None:
        conditions.append("price_per_sqm <= ?")
        conditions.append("price_per_sqm IS NOT NULL")
        params.append(max_price)
    if zone:
        conditions.append("location_zone = ?")
        params.append(zone)

    where = " AND ".join(conditions)
    params.append(limit)

    cur = conn.execute(
        f"""SELECT id, source, location, location_zone, area_sqm,
                   price_per_sqm, price_total_usd, url
            FROM listings WHERE {where}
            ORDER BY price_per_sqm ASC NULLS LAST
            LIMIT ?""",
        params,
    )

    columns = [desc[0] for desc in cur.description]
    return [dict(zip(columns, row)) for row in cur.fetchall()]


def export_data(
    conn: sqlite3.Connection, fmt: str = "json", output: Optional[str] = None
) -> None:
    """Export filtered listings+parcels for downstream use."""
    cur = conn.execute(
        """SELECT l.id, l.source, l.location, l.location_zone, l.area_sqm,
                  l.price_per_sqm, l.price_total_usd, l.url, l.is_urgent,
                  s.total_score, s.score_class, s.mode as score_mode
           FROM listings l
           LEFT JOIN scores s ON s.listing_source = l.source AND s.listing_id = l.id
           ORDER BY s.total_score DESC NULLS LAST, l.price_per_sqm ASC NULLS LAST"""
    )
    columns = [desc[0] for desc in cur.description]
    rows = [dict(zip(columns, row)) for row in cur.fetchall()]

    if fmt == "csv":
        buf = io.StringIO()
        if rows:
            writer = csv.DictWriter(buf, fieldnames=columns)
            writer.writeheader()
            writer.writerows(rows)
        result = buf.getvalue()
    else:
        result = json.dumps(rows, indent=2, ensure_ascii=False)

    if output:
        Path(output).write_text(result)
        log.info("exported %d rows to %s (%s)", len(rows), output, fmt)
    else:
        print(result)


def main() -> None:
    parser = argparse.ArgumentParser(description="Land Database — SQLite storage + ETL")
    sub = parser.add_subparsers(dest="command", required=True)

    sub.add_parser("init", help="Create/reset database schema")

    p_ll = sub.add_parser("load-listings", help="Load listings JSON")
    p_ll.add_argument("file", type=Path, help="Path to listings JSON")

    p_lp = sub.add_parser("load-parcels", help="Load parcels JSON")
    p_lp.add_argument("file", type=Path, help="Path to parcels JSON")

    sub.add_parser("load-all", help="Load all JSONs from work/data/")

    sub.add_parser("stats", help="Show database statistics")

    p_q = sub.add_parser("query", help="Query listings with filters")
    p_q.add_argument("--min-area", type=float, help="Minimum area in m²")
    p_q.add_argument("--max-price", type=float, help="Max price per m² USD")
    p_q.add_argument("--zone", type=str, help="Location zone filter")
    p_q.add_argument("--limit", type=int, default=50)

    p_e = sub.add_parser("export", help="Export data")
    p_e.add_argument("--format", choices=["json", "csv"], default="json")
    p_e.add_argument("--output", type=str, help="Output file path")

    args = parser.parse_args()

    if args.command == "init":
        conn = init_db(DB_PATH)
        conn.close()
        return

    if args.command == "load-all":
        conn = init_db(DB_PATH)
        results = load_all(conn)
        print(
            f"Loaded: {results['listings']} listings, {results['parcels']} parcels"
        )
        conn.close()
        return

    if args.command == "load-listings":
        if not args.file.exists():
            log.error("file not found: %s", args.file)
            sys.exit(1)
        conn = init_db(DB_PATH)
        n = load_listings(conn, args.file)
        print(f"Loaded {n} listings")
        conn.close()
        return

    if args.command == "load-parcels":
        if not args.file.exists():
            log.error("file not found: %s", args.file)
            sys.exit(1)
        conn = init_db(DB_PATH)
        n = load_parcels(conn, args.file)
        print(f"Loaded {n} parcels")
        conn.close()
        return

    if args.command == "stats":
        if not DB_PATH.exists():
            log.error("database not found — run 'init' first")
            sys.exit(1)
        conn = sqlite3.connect(str(DB_PATH))
        show_stats(conn)
        conn.close()
        return

    if args.command == "query":
        if not DB_PATH.exists():
            log.error("database not found — run 'init' first")
            sys.exit(1)
        conn = sqlite3.connect(str(DB_PATH))
        rows = query_listings(
            conn,
            min_area=args.min_area,
            max_price=args.max_price,
            zone=args.zone,
            limit=args.limit,
        )
        print(json.dumps(rows, indent=2, ensure_ascii=False))
        conn.close()
        return

    if args.command == "export":
        if not DB_PATH.exists():
            log.error("database not found — run 'init' first")
            sys.exit(1)
        conn = sqlite3.connect(str(DB_PATH))
        export_data(conn, fmt=args.format, output=args.output)
        conn.close()
        return


if __name__ == "__main__":
    main()
