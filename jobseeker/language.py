"""Cheap, offline language detection for job postings (en / nl / fr).

Good enough to separate English, Dutch and French postings without an extra
dependency. Claude's grade later reports the posting language too, and that
more accurate value overwrites this guess.
"""
from __future__ import annotations

import re

FR_WORDS = {
    "le", "la", "les", "des", "du", "et", "pour", "avec", "vous", "nous", "une", "est", "dans",
    "sur", "au", "aux", "qui", "que", "être", "votre", "vos", "nos", "notre", "poste", "profil",
    "entreprise", "développeur", "developpeur", "ingénieur", "ingenieur", "responsable", "technicien",
    "gestionnaire", "chef", "sécurité", "securite", "informatique", "réseau", "réseaux", "système",
    "systèmes", "administrateur", "analyste", "chargé", "conseiller", "offre", "emploi", "stage",
    "temps", "plein", "contrat", "durée", "indéterminée", "expérience", "connaissance", "connaissances",
    "h/f", "h/f/x", "f/h", "mission", "missions", "compétences", "équipe", "travail",
}
NL_WORDS = {
    "het", "een", "en", "van", "voor", "met", "je", "jij", "wij", "op", "te", "naar", "bij", "ons",
    "onze", "jouw", "functie", "functieomschrijving", "profiel", "ervaring", "kennis", "medewerker",
    "beheerder", "netwerkbeheerder", "systeembeheerder", "ontwikkelaar", "verantwoordelijke", "aanbod",
    "wat", "wie", "zoeken", "zoekt", "bent", "heb", "hebt", "werken", "bedrijf", "m/v/x", "m/v",
    "v/m/x", "jaar", "minstens", "sterke", "biedt", "graag", "tijdens", "binnen", "zijn", "deze",
    "ook", "als", "om", "hoe", "waar", "jaren", "beveiliging", "informatieveiligheid",
}
EN_WORDS = {
    "the", "and", "of", "to", "for", "with", "you", "our", "will", "your", "are", "be", "as", "an",
    "this", "that", "experience", "skills", "team", "role", "looking", "work", "knowledge", "years",
    "strong", "join", "we're", "about", "responsibilities", "requirements", "what", "who", "offer",
    "within", "working", "ability", "must", "should",
}
_ACCENTS = re.compile(r"[éèêàçùûôîâë]")
_TOKEN = re.compile(r"[a-zà-ÿ'/]+")


def detect_language(text: str) -> str:
    """Return 'en', 'nl', 'fr' or 'unknown'."""
    lowered = (text or "").lower()
    tokens = _TOKEN.findall(lowered)
    if not tokens:
        return "unknown"
    scores = {
        "fr": sum(t in FR_WORDS for t in tokens) + 0.5 * min(len(_ACCENTS.findall(lowered)), 6),
        "nl": sum(t in NL_WORDS for t in tokens),
        "en": sum(t in EN_WORDS for t in tokens),
    }
    ranked = sorted(scores.items(), key=lambda kv: kv[1], reverse=True)
    (best, top), (_, second) = ranked[0], ranked[1]
    threshold = 1 if len(tokens) < 12 else 3  # job titles are short
    if top < threshold or top <= second * 1.4:
        return "unknown"
    return best


# In Belgium "bilingual" / "tweetalig" in a job ad means Dutch + French.
_FRENCH_REQ = re.compile(
    r"\b(french|frans|franstalig\w*|français|francais|francophone|bilingual|bilingue|tweetalig\w*)\b", re.I
)


_FR = r"(?:french|frans|fran[çc]ais|fr)"
_NL_OR_EN = r"(?:dutch|nederlands|nl|english|engels|en)"
# Phrases where French is optional: "Dutch or French", "French is a plus", ...
_BILINGUAL = r"(?:bilingual|bilingue|tweetalig\w*)"
_PLUS = r"(?:plus|asset|advantage|bonus|pluspunt|troef|meerwaarde|voordeel)"
_FRENCH_OPTIONAL = [
    # "French is a plus", "Bilingual NL/FR is an asset", "Frans is een pluspunt"
    re.compile(rf"(?:\b{_BILINGUAL}\s+)?(?:\b{_NL_OR_EN}\s*/\s*)?\b{_FR}\b[^.\n]{{0,40}}?"
               rf"\b(?:is\s+)?(?:a|an|een)\s+{_PLUS}\b", re.I),
    re.compile(rf"\b(?:nice to have|pluspunt|is a plus)\b[^.\n]{{0,40}}\b{_FR}\b", re.I),
    # "Dutch or French", "French of Nederlands" (Dutch alone is enough)
    re.compile(rf"\b{_NL_OR_EN}\s+(?:or|of)\s+{_FR}\b", re.I),
    re.compile(rf"\b{_FR}\s+(?:or|of)\s+{_NL_OR_EN}\b", re.I),
]


def mentions_french(text: str) -> bool:
    """True if an English/Dutch posting asks for French (e.g. 'fluent in Dutch and French').

    'Dutch or French' and 'French is a plus' do not count: Dutch/English is enough there.
    """
    text = text or ""
    for pattern in _FRENCH_OPTIONAL:
        text = pattern.sub(" ", text)
    return bool(_FRENCH_REQ.search(text))
