"""Stage the official Codex CLI for a standalone Canvepp release."""
import base64
import hashlib
from pathlib import Path
import shutil
import sys
import tarfile
import urllib.request

VERSION = "0.169.0"
PACKAGES = {
    "win32-x64": ("x86_64-pc-windows-msvc", "nC+McFU8xRNL9tk3IhxcKdhcyOElwF/HhPO+gzSQCY7yjh3XdzSajX+MYWcHUVHWRHajfwn3/WgJYJK2wS+XWg=="),
    "darwin-arm64": ("aarch64-apple-darwin", "5WsWFBbVP78D+6FvNo1MMVxLPZdmcoQlqVJFrF4qx/TmvneLnMQKtLyXkUo54XOTDfoYKpNouTJ/F7gTVA8FnQ=="),
}
platform = sys.argv[1]
triplet, expected = PACKAGES[platform]
build = Path(__file__).resolve().parents[1] / "build" / "runtime"
build.mkdir(parents=True, exist_ok=True)
archive = build / f"codex-{VERSION}-{platform}.tgz"
if not archive.exists():
    url = f"https://registry.npmjs.org/@openai/codex/-/{archive.name}"
    pending = archive.with_suffix(".part")
    with urllib.request.urlopen(url, timeout=90) as response, pending.open("wb") as out:
        shutil.copyfileobj(response, out)
    pending.replace(archive)
with archive.open("rb") as f:
    if base64.b64encode(hashlib.file_digest(f, "sha512").digest()).decode() != expected:
        raise SystemExit(f"Codex integrity mismatch: {archive}")
dest = build / platform
with tarfile.open(archive) as tgz:
    prefix = f"package/vendor/{triplet}/"
    for member in tgz:
        if not member.isfile() or not member.name.startswith(prefix):
            continue
        relative = Path(member.name.removeprefix(prefix))
        # The app-server, its sandbox helpers and package manifest travel
        # together. The optional voice runtime is unused by this client.
        if relative.is_absolute() or ".." in relative.parts or relative.parts[:2] == ("codex-resources", "voice"):
            continue
        target = dest / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        with tgz.extractfile(member) as src, target.open("wb") as out:
            shutil.copyfileobj(src, out)
        target.chmod(member.mode & 0o755)
print(dest)
