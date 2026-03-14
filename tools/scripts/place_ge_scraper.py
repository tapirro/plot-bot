#!/usr/bin/env python3
"""Place.ge land listings scraper.

Fetches land listings from Place.ge via their XHR endpoint.
Outputs JSON to stdout or saves to file.

Usage:
    python3 tools/scripts/place_ge_scraper.py [--output FILE] [--region REGION] [--max-pages N]
    python3 tools/scripts/place_ge_scraper.py --coastal --max-pages 3

Examples:
    python3 tools/scripts/place_ge_scraper.py --output /tmp/place_ge_land.json
    python3 tools/scripts/place_ge_scraper.py --region batumi --max-pages 5
    python3 tools/scripts/place_ge_scraper.py --city-id 3  # Batumi
    python3 tools/scripts/place_ge_scraper.py --coastal --max-pages 2

Note:
    Place.ge API ignores city_id filter and returns listings from all cities.
    This scraper applies client-side filtering to match only the requested region.
"""

import argparse
import html
import json
import logging
import math
import re
import statistics
import sys
import time
from dataclasses import asdict, dataclass, field
from typing import Optional
from urllib.request import Request, urlopen
from urllib.parse import urlencode

logging.basicConfig(
    format='{"ts":"%(asctime)s","level":"%(levelname)s","msg":"%(message)s"}',
    level=logging.INFO,
)
log = logging.getLogger("place_ge")

BASE_URL = "https://place.ge/en/ads"

# Known city IDs (from window.cities on Place.ge)
CITY_IDS = {
    "tbilisi": 1,
    "rustavi": 2,
    "batumi": 3,
    "kutaisi": 4,
    "zugdidi": 5,
    "gori": 6,
    "poti": 7,
    "telavi": 8,
    "gonio": 9,
    "kobuleti": 10,
    "borjomi": 11,
    "bakuriani": 14,
    "gudauri": 15,
    "tskhaltubo": 19,
    "senaki": 21,
    "sighnaghi": 43,
    "mestia": 46,
    "stepantsminda": 53,
    "ozurgeti": 66,
    "ureki": 89,
    "chakvi": 92,
    "kvariati": 93,
    "sarpi": 94,
    "makhinjauri": 97,
}

# Adjara coastal cities for --coastal flag
COASTAL_CITIES = [
    "batumi", "kobuleti", "gonio", "ureki", "chakvi",
    "kvariati", "sarpi", "makhinjauri", "ozurgeti",
]


def _normalize_location(loc: str) -> str:
    """Extract primary city name from location string for matching."""
    # Location format: "City" or "City, District" or "City, District, Street"
    primary = loc.split(",")[0].strip()
    return primary.lower()


def _location_matches(location: str, allowed_cities: list[str]) -> bool:
    """Check if listing location matches any of the allowed city names."""
    primary = _normalize_location(location)
    for city in allowed_cities:
        city_lower = city.lower()
        if primary == city_lower:
            return True
        # Also match Georgian names that may appear
        # Handle partial matches for compound names (e.g. "shekvetili" near ureki)
    return False


def filter_by_location(
    listings: list["Listing"],
    allowed_regions: list[str],
) -> tuple[list["Listing"], int]:
    """Filter listings to only include those matching allowed regions.

    Returns (filtered_listings, removed_count).
    """
    filtered: list["Listing"] = []
    removed = 0
    for listing in listings:
        if not listing.location:
            removed += 1
            continue
        if _location_matches(listing.location, allowed_regions):
            filtered.append(listing)
        else:
            removed += 1
    return filtered, removed


def print_summary(listings: list["Listing"]) -> None:
    """Print summary statistics to stderr."""
    total = len(listings)
    if total == 0:
        print("\n--- Summary ---", file=sys.stderr)
        print("No listings to summarize.", file=sys.stderr)
        return

    prices = [l.price_total_usd for l in listings if l.price_total_usd is not None]
    per_sqm = [l.price_per_sqm_usd for l in listings if l.price_per_sqm_usd is not None]
    areas = [l.area_sqm for l in listings if l.area_sqm is not None]

    print("\n--- Summary ---", file=sys.stderr)
    print(f"Total listings: {total}", file=sys.stderr)

    if prices:
        print(f"Price range: ${min(prices):,.0f} - ${max(prices):,.0f}", file=sys.stderr)
    else:
        print("Price range: no price data", file=sys.stderr)

    if per_sqm:
        avg_psm = statistics.mean(per_sqm)
        print(f"Avg price/m²: ${avg_psm:,.1f} ({len(per_sqm)} listings with $/m² data)", file=sys.stderr)
    else:
        print("Avg price/m²: no data", file=sys.stderr)

    if areas:
        median_area = statistics.median(areas)
        print(f"Median area: {median_area:,.0f} m² (range: {min(areas):,.0f} - {max(areas):,.0f})", file=sys.stderr)
    else:
        print("Median area: no data", file=sys.stderr)

    # Status breakdown
    statuses: dict[str, int] = {}
    for l in listings:
        s = l.status or "UNKNOWN"
        statuses[s] = statuses.get(s, 0) + 1
    status_parts = ", ".join(f"{k}: {v}" for k, v in sorted(statuses.items()))
    print(f"By status: {status_parts}", file=sys.stderr)
    print("---", file=sys.stderr)


@dataclass
class Listing:
    """Single land listing from Place.ge."""
    id: int
    status: str = ""  # FOR SALE / FOR LEASE
    area_sqm: Optional[float] = None
    location: str = ""
    price_total_usd: Optional[float] = None
    price_per_sqm_usd: Optional[float] = None
    price_text: str = ""
    is_urgent: bool = False
    photo_count: int = 0
    contact: str = ""
    url: str = ""


def parse_number(s: str) -> Optional[float]:
    """Parse a number from string, handling commas."""
    s = s.replace(",", "").replace(" ", "").strip()
    try:
        return float(s)
    except (ValueError, TypeError):
        return None


def fetch_page(page: int = 1, limit: int = 100, city_id: Optional[int] = None) -> str:
    """Fetch a page of land listings from Place.ge."""
    params = {
        "object_type": "land",
        "mode": "list",
        "order_by": "date",
        "limit": limit,
        "page": page,
    }
    if city_id:
        params["city_id"] = city_id

    url = f"{BASE_URL}?{urlencode(params)}"
    req = Request(url, headers={
        "X-Requested-With": "XMLHttpRequest",
        "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
        "Accept": "text/html,application/xhtml+xml",
        "Accept-Language": "en-US,en;q=0.9",
    })

    with urlopen(req, timeout=30) as resp:
        raw = resp.read().decode("utf-8", errors="replace")
    # Response is jQuery colorbox HTML with escaped quotes
    # Unescape: \" → " and \n → newline
    raw = raw.replace('\\"', '"').replace("\\n", "\n").replace("\\t", "\t")
    return raw


def parse_listings(raw_html: str) -> list[Listing]:
    """Parse listings from Place.ge HTML response."""
    listings: list[Listing] = []

    # Extract total count
    total_match = re.search(r"Search Results:\s*([\d,]+)\s*ad", raw_html)
    total = int(total_match.group(1).replace(",", "")) if total_match else 0
    log.info(f"Total results on page: {total}")

    # Normalize
    content = raw_html.replace("\n", " ").replace("\r", "")

    # Split into listing blocks by the view link pattern
    # Each listing appears twice (image link + info link), so we need to deduplicate
    seen_ids: set[int] = set()

    # Find all infoFilter sections which contain the listing data
    # Pattern: <div class="infoFilter"> ... listing data ... </div>
    # The key fields appear as:
    #   <strong>FOR SALE</strong>, Land, 500 sq. m.<br/>Location<br/>Price: <span class="price">...
    #   tel: ..., name, role

    # Split by image+info pairs
    parts = content.split('class="photo-info"')

    for part in parts[1:]:
        # Extract listing ID
        id_match = re.search(r"/en/ads/view/(\d+)", part)
        if not id_match:
            continue
        ad_id = int(id_match.group(1))
        if ad_id in seen_ids:
            continue
        seen_ids.add(ad_id)

        listing = Listing(id=ad_id, url=f"https://place.ge/en/ads/view/{ad_id}")

        # Extract info section
        info_match = re.search(r'class="infoFilter">(.*?)(?=class="photo-info"|class="boxProdFilter"|$)', part, re.DOTALL)
        if not info_match:
            continue
        info = info_match.group(1)
        info_text = html.unescape(re.sub(r"<[^>]+>", "|", info))

        # Urgent flag
        listing.is_urgent = "urgently" in info_text.lower() or "urgent" in info_text.lower()

        # Status (FOR SALE / FOR LEASE)
        status_match = re.search(r"(FOR SALE|FOR LEASE)", info_text, re.IGNORECASE)
        if status_match:
            listing.status = status_match.group(1).upper()

        # Area
        area_match = re.search(r"([\d,]+(?:\.\d+)?)\s*sq\.\s*m", info_text)
        if area_match:
            listing.area_sqm = parse_number(area_match.group(1))

        # Location - text between area and "Price:"
        loc_match = re.search(r"sq\.\s*m\.\s*\|?\s*(.*?)\s*\|?\s*Price:", info_text)
        if loc_match:
            loc = loc_match.group(1).strip(" |,")
            loc = re.sub(r"\s*\|\s*", ", ", loc).strip(", ")
            listing.location = loc

        # Price
        price_match = re.search(r"\$([\d,]+(?:\.\d+)?)\s*/\s*\$([\d,]+(?:\.\d+)?)\s*sq", info_text)
        if price_match:
            listing.price_total_usd = parse_number(price_match.group(1))
            listing.price_per_sqm_usd = parse_number(price_match.group(2))
            listing.price_text = f"${price_match.group(1)} / ${price_match.group(2)} sq.m."
        else:
            # Check for contract price or other formats
            contract_match = re.search(r"(?:Contract price|Negotiable)", info_text, re.IGNORECASE)
            if contract_match:
                listing.price_text = contract_match.group(0)
            else:
                # Single price
                single_price = re.search(r"Price:.*?\$([\d,]+(?:\.\d+)?)", info_text)
                if single_price:
                    listing.price_total_usd = parse_number(single_price.group(1))
                    listing.price_text = f"${single_price.group(1)}"

        # Photo count
        photo_match = re.search(r"(\d+)\s*photo", part)
        if photo_match:
            listing.photo_count = int(photo_match.group(1))

        # Contact
        contact_match = re.search(r"tel:\s*([\d\s-]+),?\s*([\w\s]+)?", info_text)
        if contact_match:
            listing.contact = contact_match.group(0).strip(" |")

        listings.append(listing)

    return listings


def scrape(
    max_pages: int = 50,
    city_id: Optional[int] = None,
    filter_regions: Optional[list[str]] = None,
    delay: float = 1.0,
) -> list[Listing]:
    """Scrape all land listings from Place.ge.

    Args:
        max_pages: Maximum pages to fetch.
        city_id: City ID for API hint (Place.ge may ignore this).
        filter_regions: Client-side filter - only keep listings matching these city names.
        delay: Delay between page requests.
    """
    all_listings: list[Listing] = []
    page = 1

    while page <= max_pages:
        log.info(f"Fetching page {page}...")
        try:
            raw = fetch_page(page=page, limit=100, city_id=city_id)
        except Exception as e:
            log.error(f"Failed to fetch page {page}: {e}")
            break

        listings = parse_listings(raw)
        if not listings:
            log.info(f"No listings on page {page}, stopping.")
            break

        all_listings.extend(listings)
        log.info(f"Page {page}: {len(listings)} listings (total: {len(all_listings)})")

        # Check if we got fewer than expected (last page)
        if len(listings) < 50:
            break

        page += 1
        time.sleep(delay)

    # Deduplicate by ID
    seen: set[int] = set()
    unique: list[Listing] = []
    for listing in all_listings:
        if listing.id not in seen:
            seen.add(listing.id)
            unique.append(listing)

    log.info(f"Total unique listings before filtering: {len(unique)}")

    # Client-side location filter (Place.ge API ignores city_id)
    if filter_regions:
        unique, removed = filter_by_location(unique, filter_regions)
        log.info(f"Location filter: kept {len(unique)}, removed {removed} (filter: {filter_regions})")

    return unique


def main() -> None:
    parser = argparse.ArgumentParser(description="Place.ge land listings scraper")
    parser.add_argument("--output", "-o", help="Output JSON file path")
    parser.add_argument("--region", "-r", help=f"Region name: {', '.join(sorted(CITY_IDS.keys()))}")
    parser.add_argument("--city-id", type=int, help="City ID (numeric)")
    parser.add_argument("--coastal", action="store_true",
                        help=f"Scrape all Adjara coastal cities: {', '.join(COASTAL_CITIES)}")
    parser.add_argument("--max-pages", type=int, default=50, help="Max pages to fetch (default: 50)")
    parser.add_argument("--delay", type=float, default=1.0, help="Delay between requests in seconds")
    args = parser.parse_args()

    if args.coastal and args.region:
        log.error("Cannot use --coastal and --region together")
        sys.exit(1)

    # Determine which regions to scrape and filter by
    filter_regions: Optional[list[str]] = None
    city_id = args.city_id
    region_label = args.region

    if args.coastal:
        # Scrape without city_id (get everything), filter client-side to coastal cities
        filter_regions = list(COASTAL_CITIES)
        region_label = "coastal"
        city_id = None
        log.info(f"Coastal mode: will filter to {COASTAL_CITIES}")
    elif args.region:
        region_lower = args.region.lower()
        if region_lower not in CITY_IDS:
            log.error(f"Unknown region: {args.region}. Known: {', '.join(sorted(CITY_IDS.keys()))}")
            sys.exit(1)
        city_id = CITY_IDS[region_lower]
        filter_regions = [region_lower]

    listings = scrape(
        max_pages=args.max_pages,
        city_id=city_id,
        filter_regions=filter_regions,
        delay=args.delay,
    )

    # Print summary statistics to stderr
    print_summary(listings)

    result = {
        "source": "place.ge",
        "scraped_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "filter": {
            "object_type": "land",
            "city_id": city_id,
            "region": region_label,
        },
        "total": len(listings),
        "listings": [asdict(l) for l in listings],
    }

    output = json.dumps(result, ensure_ascii=False, indent=2)

    if args.output:
        with open(args.output, "w", encoding="utf-8") as f:
            f.write(output)
        log.info(f"Saved {len(listings)} listings to {args.output}")
    else:
        print(output)


if __name__ == "__main__":
    main()
