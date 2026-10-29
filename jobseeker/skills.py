"""Skill vocabulary used to compare job postings with your CV.

Each skill has aliases (English, Dutch, common spellings) and a weight:
3 = core skill for the roles you target, 2 = solid technical skill,
1 = general / supporting. `related` links a specific tool to a broader skill
you do have (e.g. Splunk -> SIEM), which counts as a partial match.

Whether *you* have a skill is decided by searching your CV.pdf for its aliases,
so updating your CV updates the matching automatically.
"""
from __future__ import annotations

import re
from dataclasses import dataclass, field


@dataclass(frozen=True)
class Skill:
    name: str
    aliases: tuple[str, ...]
    weight: int = 2
    related: tuple[str, ...] = field(default_factory=tuple)


def S(name: str, *aliases: str, w: int = 2, rel: tuple[str, ...] = ()) -> Skill:
    return Skill(name, tuple(a.lower() for a in (name, *aliases)), w, rel)


SKILLS: list[Skill] = [
    # --- offensive security
    S("Penetration testing", "penetration test", "penetration tester", "pentest", "pentesting", "pentester",
      "pen test", "pen testing", "ethical hacking", "ethical hacker", "offensive security", w=3),
    S("Red teaming", "red team", "red teamer", w=2, rel=("Penetration testing",)),
    S("Bug bounty", "hackerone", "bugcrowd", w=1),
    S("Metasploit", w=2), S("Burp Suite", "burp", w=2), S("Nmap", w=2),
    S("OWASP", "owasp top 10", w=2),
    S("Web application security", "web app security", "application security", "appsec", w=2,
      rel=("OWASP", "Burp Suite")),
    S("Kali Linux", "kali", w=1, rel=("Metasploit", "Nmap")),
    S("BloodHound", w=1, rel=("Active Directory",)),
    S("OSCP", w=2), S("CEH", w=1), S("CISSP", w=2), S("CompTIA Security+", "security+", w=1), S("CISM", w=1),
    # --- vulnerability management
    S("Vulnerability management", "vulnerability scanning", "vulnerability assessment", "vulnerability scan",
      "kwetsbaarheden", "kwetsbaarheidsbeheer", w=3),
    S("Nessus", "tenable", w=2), S("OpenVAS", "greenbone", w=2),
    S("Qualys", w=1, rel=("Vulnerability management",)),
    S("Rapid7", "insightvm", w=1, rel=("Vulnerability management",)),
    S("Risk assessment", "risk scoring", "risk analysis", "risicoanalyse", "risk management", w=2),
    S("Security reporting", "remediation report", "remediation reports", "hardening report", w=1),
    # --- defensive security
    S("Active Directory", "ad security", "entra id", "azure ad", w=3),
    S("Firewall management", "firewall", "firewalls", w=2),
    S("Fortinet", "fortigate", w=1, rel=("Firewall management",)),
    S("Palo Alto", "palo alto networks", w=1, rel=("Firewall management",)),
    S("Check Point", "checkpoint", w=1, rel=("Firewall management",)),
    S("SIEM", w=3),
    S("Splunk", w=2, rel=("SIEM",)), S("Microsoft Sentinel", "azure sentinel", "sentinel", w=2, rel=("SIEM",)),
    S("QRadar", w=1, rel=("SIEM",)), S("Elastic Stack", "elk", "elasticsearch", w=1, rel=("SIEM",)),
    S("IDS/IPS", "ids", "ips", "intrusion detection", "intrusion prevention", "suricata", "snort", w=2),
    S("EDR", "xdr", "crowdstrike", "defender for endpoint", "sentinelone", w=2, rel=("Incident response",)),
    S("SOC", "security operations center", "security operations centre", "soc analyst", w=3,
      rel=("SIEM", "Incident response")),
    S("Incident response", "incident handling", "incident management", "csirt", "cert", w=3),
    S("Threat hunting", "threat intelligence", "threat intel", "mitre att&ck", "mitre attack", w=2,
      rel=("SIEM",)),
    S("Digital forensics", "forensics", "dfir", w=1, rel=("Incident response",)),
    S("Malware analysis", "reverse engineering", w=1),
    S("VPN", "wireguard", "ipsec", w=1),
    S("Wireshark", "packet analysis", "tcpdump", w=2),
    S("Network security", "netwerkbeveiliging", "network segmentation", w=2, rel=("Firewall management",)),
    S("Hardening", "system hardening", "security hardening", "cis benchmarks", w=2),
    S("IAM", "identity and access management", "identity & access management", "pam", "cyberark", "okta",
      w=2, rel=("Active Directory",)),
    S("Zero trust", w=1),
    S("ISO 27001", "iso27001", "iso/iec 27001", w=2),
    S("NIS2", "nis 2", "nis-2", w=1), S("GDPR", "avg", w=1), S("NIST", w=1),
    S("Cloud security", w=2), S("OT security", "ics", "scada", w=1),
    # --- development
    S("Python", w=3), S("JavaScript", w=2), S("TypeScript", w=2), S("PHP", w=1), S("C#", w=2),
    S("SQL", "mysql", "postgresql", "postgres", w=1),
    S("Java", w=2), S("Go", "golang", w=1), S("Rust", w=1), S("C++", w=1), S(".NET", "dotnet", "asp.net", w=2,
      rel=("C#",)),
    S("Bash", "shell scripting", w=1, rel=("Linux",)), S("PowerShell", w=1),
    S("React", "react.js", "reactjs", w=2), S("Next.js", "nextjs", w=2, rel=("React",)),
    S("Node.js", "nodejs", "node", w=2, rel=("JavaScript",)),
    S("Angular", w=1, rel=("TypeScript",)), S("Vue", "vue.js", "vuejs", w=1, rel=("JavaScript",)),
    S("Three.js", "threejs", "webgl", w=1), S("Supabase", w=1, rel=("SQL",)),
    S("Full stack development", "full stack", "full-stack", "fullstack", w=3),
    S("Web development", "web developer", "webontwikkeling", "front-end", "frontend", "back-end", "backend",
      w=2),
    S("REST APIs", "rest api", "api development", "restful", w=1),
    S("SEO", w=1),
    S("Git", "github", "gitlab", w=1),
    S("CI/CD", "ci-cd", "continuous integration", "github actions", "jenkins", "gitlab ci", w=1, rel=("Git",)),
    S("DevSecOps", w=2, rel=("Penetration testing", "CI/CD")),
    S("Docker", "containers", "containerisation", "containerization", w=2),
    S("Kubernetes", "k8s", "openshift", w=1, rel=("Docker",)),
    S("Terraform", "infrastructure as code", "iac", w=1),
    S("Ansible", w=1),
    S("Machine learning", "ai/ml", "deep learning", w=1),
    S("Computer vision", "opencv", w=1, rel=("Python",)),
    # --- cloud & infra
    S("AWS", "amazon web services", "s3", "sqs", "ec2", "lambda", w=2),
    S("Azure", "microsoft azure", w=2, rel=("AWS",)), S("GCP", "google cloud", w=1, rel=("AWS",)),
    S("Linux", "ubuntu", "debian", "red hat", "rhel", w=2),
    S("Windows Server", "windows", w=1),
    S("Networking", "tcp/ip", "netwerken", "routing", "switching", "ccna", w=1),
    S("Cisco", w=1, rel=("Networking",)),
    S("Node-RED", "node red", w=1),
    # --- hardware & IoT
    S("IoT", "internet of things", "iot security", w=3),
    S("Embedded systems", "embedded", "embedded software", w=2),
    S("Firmware", w=2), S("PCB design", "pcb", w=1), S("Soldering", "solderen", w=1),
    S("Arduino", w=1, rel=("Embedded systems",)), S("Raspberry Pi", w=1, rel=("Embedded systems",)),
    S("Sensors", "sensor", "sensoren", w=1),
    S("Drones", "drone", "uav", w=1),
    # --- other
    S("Teaching", "lecturer", "docent", "training", "trainer", "coaching", "mentoring", w=1),
    S("Agile", "scrum", "kanban", w=1),
]

SKILL_BY_NAME = {s.name: s for s in SKILLS}

# Aliases that are too ambiguous to search in free text on their own.
_SHORT_SAFE = {"ids", "ips", "go", "pam", "ics", "cert", "node", "avg", "ad security", "iac", "s3", "elk"}
_PATTERNS: dict[str, re.Pattern[str]] = {}


def _pattern(skill: Skill) -> re.Pattern[str]:
    if skill.name not in _PATTERNS:
        parts = []
        for alias in skill.aliases:
            esc = re.escape(alias)
            if alias in _SHORT_SAFE:
                # Short aliases only count in upper case in the original text (IDS, GO, PAM...).
                continue
            parts.append(rf"(?<![a-z0-9]){esc}(?![a-z0-9+#])")
        _PATTERNS[skill.name] = re.compile("|".join(parts), re.I) if parts else re.compile(r"(?!x)x")
    return _PATTERNS[skill.name]


_SHORT_UPPER = {
    "IDS/IPS": re.compile(r"\b(?:IDS|IPS)\b"),
    "Go": re.compile(r"\bGo\b(?=\s*(?:,|/|\)|and|or|en|developer|programming|language|lang))|\bGolang\b"),
    "IAM": re.compile(r"\bPAM\b"),
    "OT security": re.compile(r"\bICS\b"),
    "Incident response": re.compile(r"\bCERT\b"),
    "Node.js": re.compile(r"\bNode\b(?!-RED|\s*RED)"),
    "GDPR": re.compile(r"\bAVG\b"),
    "Terraform": re.compile(r"\bIaC\b"),
    "AWS": re.compile(r"\bS3\b"),
    "Elastic Stack": re.compile(r"\bELK\b"),
}


def find_skills(text: str) -> set[str]:
    """Names of all skills mentioned in `text`."""
    found = set()
    for skill in SKILLS:
        if _pattern(skill).search(text):
            found.add(skill.name)
    for name, pat in _SHORT_UPPER.items():
        if pat.search(text):
            found.add(name)
    return found
