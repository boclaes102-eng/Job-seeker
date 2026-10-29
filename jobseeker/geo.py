"""Rough distances between Belgian places, without any online service.

Job locations come in Dutch, French or English ("Leuven" / "Louvain") and
sometimes as a postcode ("3020 Herent"). Known town names are used first,
then the postcode region. Accuracy is a few km to ~20 km, which is enough to
tell "next door" from "other side of the country".
"""
from __future__ import annotations

import math
import re

# name -> (lat, lon). Several language variants map to the same place.
PLACES: dict[str, tuple[float, float]] = {}


def _add(coords: tuple[float, float], *names: str) -> None:
    for n in names:
        PLACES[n.lower()] = coords


_add((50.99, 4.87), "langdorp")
_add((50.99, 4.84), "aarschot")
_add((50.88, 4.70), "leuven", "louvain", "heverlee", "kessel-lo", "wilsele")
_add((50.90, 4.66), "herent")
_add((50.86, 4.69), "haasrode")
_add((50.81, 4.94), "tienen", "tirlemont")
_add((50.99, 5.05), "diest")
_add((51.04, 4.87), "westerlo")
_add((50.85, 4.35), "brussels", "brussel", "bruxelles", "saint-gilles", "sint-gillis", "ixelles", "elsene",
     "etterbeek", "anderlecht", "schaerbeek", "schaarbeek", "evere", "uccle", "ukkel", "woluwe",
     "molenbeek", "laeken", "laken", "forest", "vorst", "auderghem", "oudergem")
_add((50.88, 4.47), "diegem", "machelen", "zaventem", "zellik", "vilvoorde", "vilvorde", "dilbeek",
     "grimbergen", "kortenberg", "groot-bijgaarden", "asse", "wemmel", "strombeek")
_add((50.73, 4.24), "halle", "hal")
_add((50.72, 4.61), "wavre", "waver", "ottignies", "louvain-la-neuve", "nijvel", "nivelles", "waterloo")
_add((51.22, 4.40), "antwerp", "antwerpen", "anvers", "berchem", "deurne", "wilrijk", "merksem", "hoboken")
_add((51.17, 4.46), "mortsel", "edegem", "kontich", "aartselaar", "boechout", "wommelgem")
_add((51.13, 4.57), "lier", "lierre")
_add((51.03, 4.48), "mechelen", "malines")
_add((51.09, 4.37), "boom", "willebroek")
_add((51.32, 4.94), "turnhout")
_add((51.16, 4.99), "geel")
_add((51.19, 4.83), "herentals")
_add((51.17, 5.17), "mol")
_add((50.93, 5.34), "hasselt")
_add((50.96, 5.50), "genk")
_add((51.05, 5.22), "beringen")
_add((50.78, 5.46), "tongeren", "tongres")
_add((50.98, 5.48), "diepenbeek")
_add((51.05, 3.72), "ghent", "gent", "gand", "zwijnaarde", "merelbeke")
_add((50.94, 4.04), "aalst", "alost")
_add((51.17, 4.14), "sint-niklaas", "saint-nicolas")
_add((51.03, 4.10), "dendermonde", "termonde")
_add((51.21, 3.22), "bruges", "brugge")
_add((50.83, 3.27), "kortrijk", "courtrai")
_add((50.95, 3.12), "roeselare", "roulers")
_add((51.23, 2.92), "ostend", "oostende", "ostende")
_add((51.06, 3.38), "wingene", "tielt")
_add((50.85, 2.88), "ypres", "ieper")
_add((50.63, 5.57), "liège", "liege", "luik", "seraing", "ans", "herstal")
_add((50.47, 4.87), "namur", "namen")
_add((50.41, 4.44), "charleroi", "gosselies")
_add((50.45, 3.95), "mons", "bergen")
_add((50.61, 3.39), "tournai", "doornik")
_add((49.68, 5.82), "arlon", "aarlen")
_add((49.85, 5.70), "luxembourg", "luxemburg", "bastogne", "marche-en-famenne", "libramont")
_add((50.10, 4.96), "dinant")
_add((50.71, 3.24), "mouscron", "moeskroen")
_add((50.68, 4.37), "braine-l'alleud", "eigenbrakel", "braine-l’alleud")
_add((50.60, 4.33), "braine-le-comte", "s-gravenbrakel")
_add((50.70, 4.40), "la hulpe", "terhulpen", "rixensart", "genval")
_add((50.77, 4.27), "beersel", "drogenbos", "ruisbroek")
_add((50.94, 4.02), "erembodegem")
_add((51.20, 4.23), "beveren", "beveren-waas")
_add((51.10, 5.16), "meerhout", "balen")
_add((50.87, 4.52), "nossegem", "sterrebeek", "kraainem", "wezembeek-oppem")
_add((50.83, 4.49), "tervuren", "overijse")
_add((50.93, 4.43), "vilvoorde", "haren", "neder-over-heembeek")
_add((51.08, 4.71), "heist-op-den-berg", "putte", "bonheiden")
_add((51.02, 4.69), "keerbergen", "tremelo", "rotselaar", "haacht")
_add((50.97, 4.98), "scherpenheuvel", "scherpenheuvel-zichem", "zichem", "averbode")
_add((50.79, 4.30), "sint-genesius-rode", "rhode-saint-genèse", "linkebeek")

# Postcode ranges -> approximate centre of the region.
POSTCODE_REGIONS: list[tuple[int, int, tuple[float, float]]] = [
    (1000, 1299, (50.85, 4.35)),   # Brussels
    (1300, 1499, (50.68, 4.57)),   # Walloon Brabant
    (1500, 1999, (50.88, 4.40)),   # Halle-Vilvoorde
    (2000, 2299, (51.21, 4.42)),   # Antwerp
    (2300, 2499, (51.25, 4.95)),   # Kempen
    (2500, 2699, (51.15, 4.50)),   # Lier / Antwerp south
    (2800, 2899, (51.03, 4.48)),   # Mechelen
    (2900, 2999, (51.30, 4.50)),   # Antwerp north
    (3000, 3199, (50.88, 4.70)),   # Leuven
    (3200, 3299, (50.99, 4.90)),   # Aarschot / Diest
    (3300, 3499, (50.81, 4.94)),   # Tienen / Landen
    (3500, 3999, (50.95, 5.40)),   # Limburg
    (4000, 4999, (50.60, 5.65)),   # Liège
    (5000, 5999, (50.40, 4.90)),   # Namur
    (6000, 6599, (50.41, 4.44)),   # Charleroi
    (6600, 6999, (49.95, 5.50)),   # Luxembourg province
    (7000, 7999, (50.50, 3.90)),   # Hainaut
    (8000, 8999, (51.00, 3.10)),   # West Flanders
    (9000, 9999, (51.03, 3.80)),   # East Flanders
]

_POSTCODE = re.compile(r"\b([1-9]\d{3})\b")
_REMOTE = re.compile(r"(fully remote|100% remote|remote[- ]first|volledig thuiswerk|work from home|"
                     r"full remote|t[ée]l[ée]travail complet|100% t[ée]l[ée]travail)", re.I)
_HYBRID = re.compile(r"\b(hybrid|hybride|thuiswerk|telewerk|remote|t[ée]l[ée]travail)\b", re.I)


_NAMES_LONGEST_FIRST = sorted(PLACES, key=len, reverse=True)


def locate_all(text: str | None) -> list[tuple[float, float]]:
    """Coordinates of every known place in a location string ("Leuven, Louvain-la-Neuve")."""
    if not text:
        return []
    low = text.lower()
    found: list[tuple[float, float]] = []
    for name in _NAMES_LONGEST_FIRST:  # longest first so "louvain-la-neuve" beats "louvain"
        pattern = rf"(?<![a-zà-ÿ-]){re.escape(name)}(?![a-zà-ÿ-])"
        if re.search(pattern, low):
            found.append(PLACES[name])
            low = re.sub(pattern, " ", low)
    if not found:
        for m in _POSTCODE.finditer(text):
            code = int(m.group(1))
            found += [coords for lo, hi, coords in POSTCODE_REGIONS if lo <= code <= hi]
    return found


def locate(text: str | None) -> tuple[float, float] | None:
    places = locate_all(text)
    return places[0] if places else None


def distance_km(a: tuple[float, float], b: tuple[float, float]) -> float:
    lat1, lon1, lat2, lon2 = map(math.radians, (*a, *b))
    h = math.sin((lat2 - lat1) / 2) ** 2 + math.cos(lat1) * math.cos(lat2) * math.sin((lon2 - lon1) / 2) ** 2
    road_factor = 1.25  # straight line -> rough road distance
    return 2 * 6371 * math.asin(math.sqrt(h)) * road_factor


def work_mode(text: str) -> str:
    """'remote', 'hybrid' or 'onsite/unknown' from the posting text."""
    if _REMOTE.search(text or ""):
        return "remote"
    if _HYBRID.search(text or ""):
        return "hybrid"
    return "unknown"
