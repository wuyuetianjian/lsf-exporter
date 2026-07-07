#!/bin/sh
# Emit exporter extension JSON only. Send diagnostics to stderr.

python3 - <<'PY'
import json
import re
import shutil
import subprocess
import sys


def run(command):
    executable = command[0]
    if shutil.which(executable) is None:
        print(f"{executable} not found in PATH", file=sys.stderr)
        return []
    try:
        output = subprocess.check_output(command, text=True, stderr=subprocess.PIPE)
        return [line.strip() for line in output.splitlines() if line.strip()]
    except subprocess.CalledProcessError as exc:
        detail = exc.stderr.strip() if exc.stderr else str(exc)
        print(f"{' '.join(command)} failed: {detail}", file=sys.stderr)
        return []


def to_int(value):
    value = str(value).strip()
    if value in ("", "-", "unlimited", "UNLIMITED"):
        return 0
    try:
        return int(float(value))
    except ValueError:
        return 0


def to_float(value):
    value = str(value).strip()
    if value in ("", "-"):
        return 0.0
    try:
        return float(value)
    except ValueError:
        return 0.0


def bool_from_status(status, keyword):
    return keyword.lower() in status.lower()


queues = []
queue_cmd = [
    "bqueues",
    "-o",
    "queue_name status priority njobs pend run ssusp ususp max",
    "-noheader",
]
for line in run(queue_cmd):
    parts = line.split()
    if len(parts) < 8:
        print(f"skip unparsable bqueues line: {line}", file=sys.stderr)
        continue

    name, status, priority, num_jobs, pending, running, ssusp, ususp = parts[:8]
    max_jobs = parts[8] if len(parts) > 8 else "0"
    queues.append(
        {
            "name": name,
            "status": status,
            "priority": to_int(priority),
            "open": bool_from_status(status, "open"),
            "active": bool_from_status(status, "active"),
            "max_jobs": to_int(max_jobs),
            "num_jobs": to_int(num_jobs),
            "pending": to_int(pending),
            "running": to_int(running),
            "suspended": to_int(ssusp) + to_int(ususp),
        }
    )


hosts = []
host_cmd = [
    "bhosts",
    "-o",
    "host_name status max njobs run ssusp ususp",
    "-noheader",
]
for line in run(host_cmd):
    parts = line.split()
    if len(parts) < 7:
        print(f"skip unparsable bhosts line: {line}", file=sys.stderr)
        continue

    name, status, max_jobs, num_jobs, running, ssusp, ususp = parts[:7]
    hosts.append(
        {
            "name": name,
            "status": status,
            "closed": status.lower() not in ("ok", "open"),
            "max_jobs": to_int(max_jobs),
            "num_jobs": to_int(num_jobs),
            "running": to_int(running),
            "suspended": to_int(ssusp) + to_int(ususp),
        }
    )


licenses = []
for line in run(["blstat"]):
    lower = line.lower()
    if lower.startswith("feature") or lower.startswith("-"):
        continue
    parts = re.split(r"\s+", line)
    if len(parts) < 4:
        continue
    feature = parts[0]
    numbers = [to_float(value) for value in parts[1:] if re.match(r"^-?\d+(\.\d+)?$", value)]
    if len(numbers) >= 2:
        total = numbers[0]
        used = numbers[1]
        licenses.append(
            {
                "feature": feature,
                "total": total,
                "used": used,
                "free": max(total - used, 0.0),
            }
        )


json.dump(
    {
        "queues": queues,
        "hosts": hosts,
        "licenses": licenses,
    },
    sys.stdout,
    separators=(",", ":"),
)
sys.stdout.write("\n")
PY
