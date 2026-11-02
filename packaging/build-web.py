"""Stage the documentation for Cloudflare Pages deployment."""
from pathlib import Path
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[1]
DEST = ROOT / "dist" / "web"

if DEST.exists():
    shutil.rmtree(DEST)
shutil.copytree(ROOT / "docs", DEST)
revision = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
(DEST / "build-revision.txt").write_text(revision + "\n")
print(f"Staged documentation in {DEST}")
