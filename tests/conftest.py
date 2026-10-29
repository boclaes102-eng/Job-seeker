"""Point the app at a throwaway data folder and a generated CV before it is imported.

CONFIG is read at import time, so the environment must be set here, at the top
of conftest, not inside a fixture. Your real CV.pdf and data/ are never touched.
"""
import os
import tempfile
from pathlib import Path

import pymupdf

_TMP = Path(tempfile.mkdtemp(prefix="jobseeker-tests-"))
CV_TEXT = (
    "Security engineer. Penetration testing, OWASP, Burp Suite, Nmap, Metasploit. "
    "SIEM, incident response, vulnerability management, Active Directory. "
    "Python, JavaScript, React, SQL, Linux, Docker."
)


def _make_cv(path: Path) -> None:
    doc = pymupdf.open()
    doc.new_page().insert_textbox(pymupdf.Rect(50, 50, 550, 800), CV_TEXT)
    doc.save(path)
    doc.close()


_make_cv(_TMP / "CV.pdf")
os.environ["DATA_DIR"] = str(_TMP / "data")
os.environ["CV_PDF"] = str(_TMP / "CV.pdf")
os.environ["AUTO_SEARCH_HOURS"] = "0"
for key in ("JOOBLE_API_KEY", "ADZUNA_APP_ID", "ADZUNA_APP_KEY"):
    os.environ[key] = ""  # never call paid or keyed APIs from tests
