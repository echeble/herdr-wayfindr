#!/usr/bin/env python3
"""
verify-semver.py
Strict Semantic Versioning & Sync Linter for Wayfindr.

Checks:
1. Valid SemVer in herdr-plugin.toml (MAJOR.MINOR.PATCH).
2. If code changed (cmd/, internal/, herdr/, go.mod, go.sum), version must be incremented.
3. Version increment must be strictly valid SemVer (patch, minor, or major).
4. README.md badge must match the new version.
5. CHANGELOG.md must contain an entry for the new version.
"""

import argparse
import os
import re
import subprocess
import sys

CODE_PATH_PREFIXES = (
    "cmd/",
    "internal/",
    "herdr/",
    "go.mod",
    "go.sum",
    "config.example.toml",
    "herdr-plugin.toml",
)

def emit_github(level: str, message: str, file: str = None, line: int = None):
    """Emit GitHub Actions workflow command annotations if in CI."""
    if os.environ.get("GITHUB_ACTIONS") == "true":
        loc = ""
        if file:
            loc += f" file={file}"
        if line:
            loc += f",line={line}"
        print(f"::{level}{loc}::{message}")
    else:
        prefix = {
            "error": "❌ ERROR:",
            "warning": "⚠️  WARNING:",
            "notice": "ℹ️  NOTICE:",
        }.get(level, f"{level.upper()}:")
        print(f"{prefix} {message}", file=sys.stderr if level == "error" else sys.stdout)

def parse_semver(v: str):
    if not v:
        return None
    m = re.match(r"^([0-9]+)\.([0-9]+)\.([0-9]+)$", v.strip())
    if not m:
        return None
    return tuple(map(int, m.groups()))

def format_semver(parsed_tuple) -> str:
    return ".".join(map(str, parsed_tuple))

def extract_manifest_version(content: str):
    if not content:
        return None
    m = re.search(r"^version\s*=\s*\"([^\"]+)\"", content, re.MULTILINE)
    return m.group(1).strip() if m else None

def get_git_file(ref: str, path: str):
    try:
        res = subprocess.check_output(
            ["git", "show", f"{ref}:{path}"],
            text=True,
            stderr=subprocess.DEVNULL,
        )
        return res
    except Exception:
        return None

def get_changed_files(base_ref: str, head_ref: str = "HEAD"):
    try:
        out = subprocess.check_output(
            ["git", "diff", "--name-only", f"{base_ref}...{head_ref}"],
            text=True,
            stderr=subprocess.DEVNULL,
        )
        return [line.strip() for line in out.strip().splitlines() if line.strip()]
    except Exception:
        return []

def main():
    parser = argparse.ArgumentParser(description="Verify SemVer and changelog consistency.")
    parser.add_argument("--base", default="origin/master", help="Base git ref to compare against (default: origin/master)")
    parser.add_argument("--head", default="HEAD", help="Head git ref (default: HEAD)")
    parser.add_argument("--repo-root", default=".", help="Repository root directory")
    args = parser.parse_args()

    repo_root = os.path.abspath(args.repo_root)
    manifest_path = os.path.join(repo_root, "herdr-plugin.toml")
    readme_path = os.path.join(repo_root, "README.md")
    changelog_path = os.path.join(repo_root, "CHANGELOG.md")

    errors = []
    notices = []

    # 1. Read current herdr-plugin.toml
    if not os.path.exists(manifest_path):
        emit_github("error", "herdr-plugin.toml not found!", file="herdr-plugin.toml")
        sys.exit(1)

    with open(manifest_path, "r", encoding="utf-8") as f:
        head_manifest_content = f.read()

    head_version_str = extract_manifest_version(head_manifest_content)
    if not head_version_str:
        errors.append("Could not find 'version = \"...\"' in herdr-plugin.toml")
        emit_github("error", "Missing version field in herdr-plugin.toml", file="herdr-plugin.toml")
        sys.exit(1)

    head_semver = parse_semver(head_version_str)
    if not head_semver:
        errors.append(f"Invalid SemVer in herdr-plugin.toml: '{head_version_str}'. Must follow MAJOR.MINOR.PATCH format.")
        emit_github("error", f"Invalid SemVer: '{head_version_str}'", file="herdr-plugin.toml")

    # 2. Get base version
    base_manifest_content = get_git_file(args.base, "herdr-plugin.toml")
    base_version_str = extract_manifest_version(base_manifest_content) if base_manifest_content else None
    base_semver = parse_semver(base_version_str) if base_version_str else None

    # 3. Inspect changed files
    changed_files = get_changed_files(args.base, args.head)
    code_changed_files = [
        f for f in changed_files if any(f.startswith(p) if p.endswith("/") else f == p for p in CODE_PATH_PREFIXES)
    ]
    has_code_changes = len(code_changed_files) > 0

    # 4. SemVer Bump Analysis
    bump_type = "none"
    valid_bump = False
    expected_patch = None
    expected_minor = None
    expected_major = None

    if base_semver and head_semver:
        b_maj, b_min, b_pat = base_semver
        h_maj, h_min, h_pat = head_semver

        expected_patch = f"{b_maj}.{b_min}.{b_pat + 1}"
        expected_minor = f"{b_maj}.{b_min + 1}.0"
        expected_major = f"{b_maj + 1}.0.0"

        if h_maj == b_maj + 1 and h_min == 0 and h_pat == 0:
            bump_type = "major"
            valid_bump = True
        elif h_maj == b_maj and h_min == b_min + 1 and h_pat == 0:
            bump_type = "minor"
            valid_bump = True
        elif h_maj == b_maj and h_min == b_min and h_pat == b_pat + 1:
            bump_type = "patch"
            valid_bump = True
        elif head_semver == base_semver:
            bump_type = "none"
            valid_bump = False
        else:
            bump_type = "invalid"
            valid_bump = False

        if has_code_changes:
            if bump_type == "none":
                err = (
                    f"Code changes detected ({len(code_changed_files)} files), but version was not bumped from {base_version_str}!\n"
                    f"  Expected next version:\n"
                    f"    - Patch: {expected_patch}\n"
                    f"    - Minor: {expected_minor}\n"
                    f"    - Major: {expected_major}"
                )
                errors.append(err)
                emit_github("error", err, file="herdr-plugin.toml")
            elif bump_type == "invalid":
                err = (
                    f"Version '{head_version_str}' is not a valid SemVer increment over base '{base_version_str}'.\n"
                    f"  Allowed next versions:\n"
                    f"    - Patch: {expected_patch}\n"
                    f"    - Minor: {expected_minor}\n"
                    f"    - Major: {expected_major}"
                )
                errors.append(err)
                emit_github("error", err, file="herdr-plugin.toml")
        else:
            if bump_type == "invalid":
                err = f"Version '{head_version_str}' is not a valid SemVer increment over base '{base_version_str}'."
                errors.append(err)
                emit_github("error", err, file="herdr-plugin.toml")
    elif not base_semver:
        notices.append(f"Could not resolve base version on ref '{args.base}'. Skipping relative bump check.")

    # 5. Check README.md
    if os.path.exists(readme_path):
        with open(readme_path, "r", encoding="utf-8") as f:
            readme_content = f.read()
        badge_pattern = rf"badge/herdr-wayfindr-v{re.escape(head_version_str)}-blue"
        if not re.search(badge_pattern, readme_content):
            err = f"README.md version badge does not match herdr-plugin.toml version ({head_version_str}). Expected badge: 'herdr-wayfindr-v{head_version_str}'."
            errors.append(err)
            emit_github("error", err, file="README.md")
    else:
        notices.append("README.md not found.")

    # 6. Check CHANGELOG.md
    if os.path.exists(changelog_path):
        with open(changelog_path, "r", encoding="utf-8") as f:
            changelog_content = f.read()
        changelog_pattern = rf"^##\s+\[{re.escape(head_version_str)}\]"
        if not re.search(changelog_pattern, changelog_content, re.MULTILINE):
            err = f"CHANGELOG.md is missing an entry for version [{head_version_str}]. Expected header: '## [{head_version_str}] - YYYY-MM-DD'."
            errors.append(err)
            emit_github("error", err, file="CHANGELOG.md")
    else:
        notices.append("CHANGELOG.md not found.")

    # 7. Output Step Summary for GitHub Actions
    summary_file = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary_file:
        status_icon = "❌ Failed" if errors else "✅ Passed"
        bump_display = {
            "major": "🚀 Major bump",
            "minor": "✨ Minor bump",
            "patch": "🩹 Patch bump",
            "none": "Unchanged",
            "invalid": "⚠️ Invalid bump",
        }.get(bump_type, bump_type)

        md = []
        md.append(f"## 🏷️ SemVer & Changelog Check: {status_icon}\n")
        md.append("| Metric | Status | Details |")
        md.append("| :--- | :--- | :--- |")
        md.append(f"| **Base Version** (`{args.base}`) | ℹ️ Info | `{base_version_str or 'N/A'}` |")
        md.append(f"| **PR Version** (`herdr-plugin.toml`) | {'✅ Valid' if head_semver else '❌ Invalid'} | `{head_version_str}` |")
        md.append(f"| **Bump Type** | {'✅ ' if valid_bump else ('⚠️ ' if bump_type == 'none' and not has_code_changes else '❌ ')} | {bump_display} |")
        md.append(f"| **Code Changes** | {'⚠️ Yes' if has_code_changes else 'ℹ️ No'} | {len(code_changed_files)} code file(s) changed |")
        md.append(f"| **README Badge** | {'✅ Synced' if not any('README.md' in e for e in errors) else '❌ Out of sync'} | `v{head_version_str}` |")
        md.append(f"| **CHANGELOG Entry** | {'✅ Found' if not any('CHANGELOG.md' in e for e in errors) else '❌ Missing'} | `## [{head_version_str}]` |")
        md.append("")

        if code_changed_files:
            md.append("<details><summary>Changed Code Files (" + str(len(code_changed_files)) + ")</summary>\n")
            for cf in code_changed_files[:25]:
                md.append(f"- `{cf}`")
            if len(code_changed_files) > 25:
                md.append(f"- ... and {len(code_changed_files) - 25} more")
            md.append("\n</details>\n")

        if errors:
            md.append("### ❌ Blocking Issues:\n")
            for e in errors:
                md.append(f"- {e}\n")

        with open(summary_file, "a", encoding="utf-8") as sf:
            sf.write("\n".join(md) + "\n")

    # Print console output
    if errors:
        print("\n" + "=" * 60, file=sys.stderr)
        print("❌ SemVer Validation Failed with the following errors:", file=sys.stderr)
        print("=" * 60, file=sys.stderr)
        for e in errors:
            print(f"- {e}", file=sys.stderr)
        print("=" * 60 + "\n", file=sys.stderr)
        sys.exit(1)
    else:
        print("\n" + "=" * 60)
        print(f"✅ SemVer validation passed! (Version: {head_version_str}, Bump: {bump_type})")
        print("=" * 60 + "\n")
        sys.exit(0)

if __name__ == "__main__":
    main()
