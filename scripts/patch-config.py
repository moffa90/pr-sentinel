#!/usr/bin/env python3
"""Requires Python 3.9+. Stdlib only.

Apply three fixes to pr-sentinel's config.yaml, in place.

One-off migration for a specific config: it names Cellgain/dms-gateway and
moffa90/pr-sentinel and expects the 4-space layout `config.Save` writes. It is
not a general config tool.

  1. labels: []  ->  labels: [pr-sentinel]   on every repo, so issues the tool
     opens are filterable and bulk-closable.
  2. Cellgain/dms-gateway: delete_branch false, require_label "auto-merge".
     Deleting the base branch of a stacked PR auto-closes the dependent one and
     GitHub will not reopen or retarget it; require_label puts a human in front
     of every auto-merge.
  3. moffa90/pr-sentinel: review_own_prs true. It was false and the repo has a
     single author, so it had never been reviewed by its own tool.

Every edit asserts its expected match count BEFORE anything is written, so a
config this script does not recognise is left untouched rather than mangled.

Labels: neither `pr-sentinel` nor `auto-merge` needs to exist beforehand.
pr-sentinel creates configured issue labels before using them, and creates a
missing auto-merge `require_label` so it can be applied (PR #7). Until someone
applies `auto-merge` to a dms-gateway PR, auto-merge there stays off.

The previous config is copied to config.yaml.bak-<UTC timestamp> (never
overwriting an older backup), and the new one is written to a temp file and
renamed into place with 0600, so the daemon's per-cycle reload never sees a
half-written file.

    python3 patch-config.py ~/.config/pr-sentinel/config.yaml
"""
import datetime
import os
import pathlib
import re
import sys
import tempfile


def repo_block(text: str, name: str) -> tuple[int, int]:
    """Return the (start, end) offsets of one `- name: <name>` repo block."""
    start = text.find(f"    - name: {name}\n")
    if start < 0:
        raise SystemExit(f"repo {name!r} not found — config not patched")
    nxt = text.find("\n    - name: ", start + 1)
    end = len(text) if nxt < 0 else nxt + 1
    # The last repo's block ends at the next top-level key, whatever it is.
    top = re.compile(r"\n[^ \n]").search(text, start)
    if top and top.start() < end:
        end = top.start() + 1
    return start, end


def edit_in_block(text: str, name: str, old: str, new: str,
                  applied: tuple[str, ...] = ()) -> tuple[str, bool]:
    """Replace `old` with `new` inside one repo block, returning the text and
    whether it changed. `applied` lists other spellings of `new` that count as
    already done (config.Save's yaml.v3 output drops quotes, for one).
    Already-applied is fine; anything else aborts before a single byte is
    written."""
    start, end = repo_block(text, name)
    block = text[start:end]
    n = block.count(old)
    if n == 0 and any(block.count(form) == 1 for form in (new, *applied)):
        return text, False  # already applied
    if n != 1:
        raise SystemExit(f"{name}: expected 1 occurrence of {old!r}, found {n} — config not patched")
    return text[:start] + block.replace(old, new, 1) + text[end:], True


def report(label: str, changed: bool) -> None:
    print(f"  {label}" if changed else f"  {label}: already set, skipping")


def write_backup(backup: pathlib.Path, text: str) -> None:
    """Create the backup 0600 from the first byte (it holds webhook URLs).
    O_EXCL refuses to overwrite an existing backup."""
    fd = os.open(backup, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    with os.fdopen(fd, "w") as f:
        f.write(text)


def write_atomic(path: pathlib.Path, text: str) -> None:
    """Write via a 0600 temp file in the same directory, then rename over path."""
    fd, tmp = tempfile.mkstemp(dir=path.parent, prefix=f".{path.name}.", suffix=".tmp")
    try:
        with os.fdopen(fd, "w") as f:
            f.write(text)
            f.flush()
            os.fsync(f.fileno())
        os.chmod(tmp, 0o600)
        os.replace(tmp, path)
    except BaseException:
        os.unlink(tmp)
        raise


def main() -> int:
    path = pathlib.Path(sys.argv[1] if len(sys.argv) > 1
                        else pathlib.Path.home() / ".config/pr-sentinel/config.yaml").expanduser()
    original = path.read_text()
    text = original

    # 1. labels on every repo. "Already applied" has two shapes: the flow form
    # this script writes, and the block form config.Save (yaml.v3) rewrites it
    # to the next time any command saves the config.
    empty = len(re.findall(r"^        labels: \[\]$", text, re.M))
    already = (len(re.findall(r"^        labels: \[pr-sentinel\]$", text, re.M))
               + len(re.findall(r"^        labels:\n            - pr-sentinel$", text, re.M)))
    if empty:
        text = re.sub(r"^        labels: \[\]$", "        labels: [pr-sentinel]", text, flags=re.M)
        print(f"  labels: [] -> [pr-sentinel]  on {empty} repos")
    if already:
        print(f"  labels: already set on {already} repos, skipping")
    if not empty and not already:
        # Not fatal: the other two edits are independent of this one.
        print("  labels: no `labels: []` to change, skipping")

    # 2. dms-gateway auto-merge guards.
    text, changed = edit_in_block(text, "Cellgain/dms-gateway",
                                  "        delete_branch: true\n", "        delete_branch: false\n")
    report("dms-gateway: delete_branch false", changed)
    text, changed = edit_in_block(text, "Cellgain/dms-gateway",
                                  '        require_label: ""\n', '        require_label: "auto-merge"\n',
                                  applied=("        require_label: auto-merge\n",))
    report("dms-gateway: require_label auto-merge", changed)

    # 3. pr-sentinel reviews its own PRs.
    text, changed = edit_in_block(text, "moffa90/pr-sentinel",
                                  "      review_own_prs: false\n", "      review_own_prs: true\n")
    report("pr-sentinel: review_own_prs true", changed)

    if text == original:
        print("nothing to change")
        return 0

    stamp = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    backup = path.with_name(f"{path.name}.bak-{stamp}")
    write_backup(backup, original)
    write_atomic(path, text)
    print(f"\npatched {path}\nbackup  {backup}\n\nNow run: pr-sentinel repos   (it validates on load)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
