#!/usr/bin/env python3
"""Build WeightMaxxer for 64-bit Windows and package both release downloads.

    python3 scripts/build.py                 # dist/ executables, both zips, SHA256SUMS.txt
    python3 scripts/build.py --version 0.9.0 --tags updatetest --out build/e2e/old --no-package

Works on Linux, macOS and Windows (Go cross-compiles; no C compiler is needed)
and from an extracted source archive without .git.
"""
import argparse
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
MODULE = "github.com/TechSavage1989/weightmaxer"
APP_ID = "com.techsavage1989.weightmaxer"
ZIP_NAME = "Weightmaxer-win-x64.zip"
BRANDED_ZIP_NAME = "WeightMaxxer-win-x64.zip"
EXE_NAME = "Weightmaxer.exe"
BRANDED_EXE_NAME = "WeightMaxxer.exe"
SUMS_NAME = "SHA256SUMS.txt"
ZIP_DOCUMENTS = [("README.txt", ROOT / "assets" / "README.txt"),
                 ("THIRD_PARTY_NOTICES.txt", ROOT / "assets" / "THIRD_PARTY_NOTICES.txt")]
FIXED_TIME = (2020, 1, 1, 0, 0, 0)  # stable zip timestamps


def run(cmd, env=None, cwd=ROOT):
    print("+", " ".join(str(c) for c in cmd), flush=True)
    subprocess.run(cmd, check=True, cwd=cwd, env=env)


def read_version():
    v = (ROOT / "VERSION").read_text(encoding="utf-8").strip()
    if not re.fullmatch(r"(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)", v):
        sys.exit(f"VERSION must be MAJOR.MINOR.PATCH, got {v!r}")
    return v


def resources(version, work):
    """Compile the icon, manifest and version info into a .syso next to main."""
    work.mkdir(parents=True, exist_ok=True)
    cfg = json.loads((ROOT / "assets" / "winres.json").read_text(encoding="utf-8"))
    cfg["RT_MANIFEST"]["#1"]["0409"]["identity"]["version"] = version + ".0"
    (work / "winres.json").write_text(json.dumps(cfg, indent=2), encoding="utf-8")
    shutil.copy(ROOT / "assets" / "icon.png", work / "icon.png")
    run(["go", "tool", "go-winres", "make", "--in", str(work / "winres.json"), "--arch", "amd64",
         "--out", str(ROOT / "cmd" / "weightmaxer" / "rsrc"),
         "--file-version", version + ".0", "--product-version", version + ".0"])


def build(version, tags, out):
    out.mkdir(parents=True, exist_ok=True)
    exe = out / BRANDED_EXE_NAME
    ldflags = (f"-s -w -H windowsgui -X {MODULE}/internal/version.Version={version} "
               f"-X '{MODULE}/internal/version.Identity=WMXID|{APP_ID}|{version}|END'")
    env = dict(os.environ, GOOS="windows", GOARCH="amd64", CGO_ENABLED="0")
    cmd = ["go", "build", "-trimpath", "-buildvcs=false", "-ldflags", ldflags, "-o", str(exe)]
    if tags:
        cmd += ["-tags", tags]
    run(cmd + ["./cmd/weightmaxer"], env=env)
    data = exe.read_bytes()
    marker = f"WMXID|{APP_ID}|{version}|END".encode()
    if marker not in data:
        sys.exit("the built executable is missing its identity marker")
    # Old installed copies and the Windows update test use the original name.
    # Both names contain the identical build and continue using the same data.
    shutil.copyfile(exe, out / EXE_NAME)
    return exe


def package(exe, version, out):
    sums = []
    for zip_name, exe_name, verifier in [
        (BRANDED_ZIP_NAME, BRANDED_EXE_NAME, "verify-branded"),
        (ZIP_NAME, EXE_NAME, "verify"),
    ]:
        zpath = out / zip_name
        with zipfile.ZipFile(zpath, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as z:
            for name, src in [(exe_name, exe), *ZIP_DOCUMENTS]:
                info = zipfile.ZipInfo(name, FIXED_TIME)
                info.compress_type = zipfile.ZIP_DEFLATED
                info.external_attr = 0o644 << 16
                z.writestr(info, src.read_bytes())
        digest = hashlib.sha256(zpath.read_bytes()).hexdigest()
        sums.append(f"{digest}  {zip_name}\n")
        # Each verifier accepts only that package's one exact executable name.
        run(["go", "run", "./cmd/wmxtool", verifier, str(zpath), version])
        print(f"{zip_name}  sha256 {digest}")
    (out / SUMS_NAME).write_text("".join(sums), encoding="utf-8", newline="\n")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--version", help="override VERSION (test builds only)")
    ap.add_argument("--tags", default="", help="Go build tags, e.g. updatetest")
    ap.add_argument("--out", default="dist")
    ap.add_argument("--no-package", action="store_true")
    a = ap.parse_args()
    version = a.version or read_version()
    out = (ROOT / a.out).resolve()
    resources(version, ROOT / "build" / "winres")
    exe = build(version, a.tags, out)
    if not a.no_package:
        package(exe, version, out)


if __name__ == "__main__":
    main()
