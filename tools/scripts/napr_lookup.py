#!/usr/bin/env python3
"""NAPR Cadastral Lookup — Georgian Property Registry API.

Queries naprweb.reestri.gov.ge for cadastral registration data.
No authentication required.

Usage:
    python3 napr_lookup.py 05.32.12.220.01.01.513
    python3 napr_lookup.py 05.32.12.220.01.01.513 05.32.12.220.01.01.515
    python3 napr_lookup.py --file codes.txt
    python3 napr_lookup.py --parcel 053212220  # ArcGIS parcel/building data
    python3 napr_lookup.py --dry-run 05.32.12.220.01.01.513

Output: JSON with registration history, parties, transaction types.

API notes:
    - NAPR: POST https://naprweb.reestri.gov.ge/api/search, no auth
    - ArcGIS: HTTP only (not HTTPS), no auth
    - appRegDate field = Unix timestamp in SECONDS
    - webTransact = string (transaction type in Georgian)
    - applicants = list of strings
    - Each applist entry = one transaction record
"""

import argparse
import json
import sys
import time
import urllib.request
import urllib.error
from datetime import datetime

NAPR_API = "https://naprweb.reestri.gov.ge/api/search"
ARCGIS_BASE = "http://gisappsn.reestri.gov.ge/ArcGIS/rest/services/CadRepGeo/MapServer"

TRANSACTION_TYPES = {
    "საკუთრების უფლების რეგისტრაცია": "ownership_registration",
    "იპოთეკის წარმოშობის რეგისტრაცია": "mortgage_registration",
    "იპოთეკის შეწყვეტის რეგისტრაცია": "mortgage_termination",
    "ამხანაგობის წევრის": "partnership_member_ownership",
}


def classify_transaction(web_transact: str) -> str:
    for ge_key, en_val in TRANSACTION_TYPES.items():
        if ge_key in web_transact:
            return en_val
    return web_transact


def ts_to_date(ts_str: str) -> str:
    try:
        ts = int(ts_str)
        if ts > 0:
            return datetime.fromtimestamp(ts).strftime("%Y-%m-%d")
    except (ValueError, OSError):
        pass
    return ""


def query_napr(cadcode: str) -> dict:
    body = json.dumps({
        "page": 1, "search": "", "regno": "",
        "datefrom": None, "dateto": None,
        "person": "", "address": "", "cadcode": cadcode
    }).encode()
    req = urllib.request.Request(
        NAPR_API, data=body,
        headers={"Content-Type": "application/json"}
    )
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            data = json.loads(resp.read().decode())
    except urllib.error.URLError as e:
        return {"cadcode": cadcode, "found": 0, "error": str(e), "transactions": []}

    apps = data.get("applist", [])
    transactions = []
    has_mortgage = False
    mortgage_terminated = False

    for app in apps:
        ttype_ge = app.get("webTransact", "")
        ttype_en = classify_transaction(ttype_ge)
        date = ts_to_date(app.get("appRegDate", ""))
        applicants = app.get("applicants", [])

        if "mortgage_registration" in ttype_en:
            has_mortgage = True
        if "mortgage_termination" in ttype_en:
            mortgage_terminated = True

        transactions.append({
            "reg_number": app.get("regNumber", ""),
            "date": date,
            "type_en": ttype_en,
            "type_ge": ttype_ge,
            "status": app.get("status", ""),
            "address": app.get("address", ""),
            "applicants": applicants,
        })

    return {
        "cadcode": cadcode,
        "found": len(transactions),
        "address": transactions[0]["address"] if transactions else "",
        "last_date": transactions[0]["date"] if transactions else "",
        "has_mortgage": has_mortgage,
        "mortgage_terminated": mortgage_terminated,
        "transactions": transactions,
    }


def query_arcgis(uniq_code: str) -> dict:
    results = {}
    for layer, name in [(14, "parcel"), (12, "building")]:
        url = (
            f"{ARCGIS_BASE}/{layer}/query?"
            f"where=UNIQ_CODE='{uniq_code}'"
            f"&outFields=*&returnGeometry=false&f=json"
        )
        try:
            with urllib.request.urlopen(url, timeout=15) as resp:
                data = json.loads(resp.read().decode())
            features = data.get("features", [])
            results[name] = [f.get("attributes", {}) for f in features]
        except urllib.error.URLError as e:
            results[name] = {"error": str(e)}
    return {"uniq_code": uniq_code, **results}


def main():
    parser = argparse.ArgumentParser(description="NAPR Cadastral Lookup")
    parser.add_argument("codes", nargs="*", help="Cadastral codes (XX.XX.XX.XXX.XX.XX.XXX)")
    parser.add_argument("--file", help="File with codes, one per line")
    parser.add_argument("--parcel", help="9-digit UNIQ_CODE for ArcGIS parcel/building query")
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--delay", type=float, default=0.5, help="Delay between requests (sec)")
    args = parser.parse_args()

    if args.parcel:
        if args.dry_run:
            print(json.dumps({"action": "arcgis_query", "uniq_code": args.parcel}, indent=2))
            return
        print(json.dumps(query_arcgis(args.parcel), indent=2, ensure_ascii=False))
        return

    codes = list(args.codes)
    if args.file:
        with open(args.file) as f:
            codes.extend(line.strip() for line in f if line.strip())

    if not codes:
        parser.print_help()
        sys.exit(1)

    if args.dry_run:
        print(json.dumps({"action": "napr_lookup", "codes": codes, "count": len(codes)}, indent=2))
        return

    results = []
    for i, code in enumerate(codes):
        result = query_napr(code)
        results.append(result)
        m = "MORTGAGE" if result["has_mortgage"] else ""
        t = "(terminated)" if result["mortgage_terminated"] else ""
        print(f"  [{i+1}/{len(codes)}] {code}: {result['found']} records {m} {t}",
              file=sys.stderr)
        if i < len(codes) - 1:
            time.sleep(args.delay)

    output = {
        "total_codes": len(codes),
        "total_found": sum(r["found"] for r in results),
        "with_mortgage": sum(1 for r in results if r["has_mortgage"]),
        "mortgage_terminated": sum(1 for r in results if r["mortgage_terminated"]),
        "results": results,
    }
    print(json.dumps(output, indent=2, ensure_ascii=False))


if __name__ == "__main__":
    main()
