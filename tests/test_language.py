import pytest

from jobseeker.language import detect_language, mentions_french


@pytest.mark.parametrize("text, lang", [
    ("We are looking for a security engineer to join our team. You will work with our SOC and "
     "you have experience with incident response and strong knowledge of networks.", "en"),
    ("Wij zoeken een security engineer voor ons team. Je hebt ervaring met netwerken en je werkt "
     "graag samen met collega's in een dynamische omgeving.", "nl"),
    ("Nous recherchons un ingénieur sécurité pour notre équipe. Vous avez une expérience en réponse "
     "aux incidents et une bonne connaissance des réseaux.", "fr"),
    ("", "unknown"),
])
def test_detect_language(text, lang):
    assert detect_language(text) == lang


@pytest.mark.parametrize("text", [
    "You are fluent in Dutch and French.",
    "Tweetalig Nederlands-Frans is vereist.",
    "Bilingual candidates only.",
])
def test_french_required(text):
    assert mentions_french(text)


@pytest.mark.parametrize("text", [
    "You speak Dutch or French.",
    "French is a plus.",
    "Frans is een pluspunt.",
    "You speak Dutch and English.",
])
def test_french_optional(text):
    assert not mentions_french(text)
