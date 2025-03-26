#!/usr/bin/env python3

import os
import re
import sys
import requests
import subprocess
from datetime import datetime

# Configuration with environment variable support
def get_rpmbuild_path():
    """Get the RPM build path, supporting different environments"""
    # First check if RPM_BUILD_ROOT environment variable is set
    if "RPM_BUILD_ROOT" in os.environ:
        return os.environ["RPM_BUILD_ROOT"]

    # For GitHub Actions running in Fedora container
    if os.path.exists("/root/rpmbuild"):
        return "/root/rpmbuild"

    # Default to user's home directory
    return os.path.expanduser("~/rpmbuild")

# Set paths based on environment
RPMBUILD_PATH = get_rpmbuild_path()
GITHUB_API_URL = "https://api.github.com/repos/zen-browser/desktop/releases/latest"
SPEC_FILE_PATH = os.path.join(RPMBUILD_PATH, "SPECS/zen-browser.spec")
SOURCES_DIR = os.path.join(RPMBUILD_PATH, "SOURCES")
SRPMS_DIR = os.path.join(RPMBUILD_PATH, "SRPMS")

def get_latest_release():
    """Get the latest release info from GitHub API"""
    response = requests.get(GITHUB_API_URL)
    if response.status_code != 200:
        print(f"Error accessing GitHub API: {response.status_code}")
        sys.exit(1)

    release_data = response.json()
    version = release_data["tag_name"]

    # Skip twilight/nightly builds (containing 't' in version)
    if 't' in version:
        print(f"Skipping twilight/nightly build version: {version}")
        sys.exit(0)

    # Find the Linux x86_64 asset
    linux_asset = None
    for asset in release_data["assets"]:
        if "linux-x86_64.tar.xz" in asset["name"]:
            linux_asset = asset
            break

    if not linux_asset:
        print("Could not find Linux x86_64 asset in the release")
        sys.exit(1)

    return {
        "version": version,
        "download_url": f"https://github.com/zen-browser/desktop/releases/download/{version}/zen.linux-x86_64.tar.xz",
        "filename": "zen.linux-x86_64.tar.xz",
        "published_at": release_data["published_at"]
    }

def update_spec_file(release_info):
    """Update the spec file with the new version"""
    with open(SPEC_FILE_PATH, "r") as f:
        spec_content = f.read()

    # Update main version
    spec_content = re.sub(r"Version:\s+.*", f"Version:        {release_info['version']}", spec_content)

    # Update Source0 URL
    source_url = f"https://github.com/zen-browser/desktop/releases/download/{release_info['version']}/zen.linux-x86_64.tar.xz"
    spec_content = re.sub(r"Source0:\s+.*", f"Source0:        {source_url}", spec_content)

    # Update desktop entry version - Fixed version
    desktop_entry_pattern = r"\[Desktop Entry\]\nVersion=.*"
    spec_content = re.sub(desktop_entry_pattern,
                         f"[Desktop Entry]\nVersion={release_info['version']}",
                         spec_content,
                         flags=re.MULTILINE)

    # Add new changelog entry
    today = datetime.now().strftime("%a %b %d %Y")
    changelog_entry = f"%changelog\n* {today} COPR Build System <copr-build@fedoraproject.org> - {release_info['version']}-1\n- Update to {release_info['version']}\n"
    spec_content = re.sub(r"%changelog.*", changelog_entry, spec_content, flags=re.DOTALL)

    # Write the updated content back
    with open(SPEC_FILE_PATH, "w") as f:
        f.write(spec_content)

    return spec_content

def download_source(release_info):
    """Download the source tarball"""
    # Ensure the SOURCES directory exists
    os.makedirs(SOURCES_DIR, exist_ok=True)

    source_path = os.path.join(SOURCES_DIR, release_info["filename"])
    response = requests.get(release_info["download_url"], stream=True)

    if response.status_code != 200:
        print(f"Error downloading source: {response.status_code}")
        sys.exit(1)

    with open(source_path, "wb") as f:
        for chunk in response.iter_content(chunk_size=8192):
            f.write(chunk)

    return source_path

def build_srpm():
    """Build the SRPM package"""
    result = subprocess.run(
        ["rpmbuild", "-bs", SPEC_FILE_PATH],
        capture_output=True,
        text=True
    )

    if result.returncode != 0:
        print(f"Error building SRPM: {result.stderr}")
        sys.exit(1)

    srpm_path = find_srpm_in_output(result)
    if not srpm_path:
        srpm_path = find_srpm_in_spec()
    if not srpm_path:
        srpm_path = find_srpm_in_directory()

    if not srpm_path:
        print("Could not find built SRPM path in output")
        print("stdout:", result.stdout)
        print("stderr:", result.stderr)
        sys.exit(1)

    print(f"Found SRPM: {srpm_path}")
    return srpm_path

def find_srpm_in_output(result):
    """Extract SRPM path from command output"""
    for line in result.stderr.split("\n"):
        if line.endswith(".src.rpm"):
            return line.strip().replace("Wrote: ", "")

    for line in result.stdout.split("\n"):
        if line.endswith(".src.rpm"):
            return line.strip().replace("Wrote: ", "")

    return None

def find_srpm_in_spec():
    """Find SRPM based on spec file version info"""
    with open(SPEC_FILE_PATH, "r") as f:
        spec_content = f.read()
        version_match = re.search(r"Version:\s+(.*)", spec_content)
        release_match = re.search(r"Release:\s+(.*)", spec_content)

        if version_match and release_match:
            version = version_match.group(1)
            release = release_match.group(1).replace("%{?dist}", ".fc41")
            expected_path = os.path.join(SRPMS_DIR, f"zen-browser-{version}-{release}.src.rpm")
            if os.path.exists(expected_path):
                return expected_path
    return None

def find_srpm_in_directory():
    """Find most recent SRPM in SRPMS directory"""
    try:
        os.makedirs(SRPMS_DIR, exist_ok=True)
        for file in os.listdir(SRPMS_DIR):
            if file.endswith(".src.rpm"):
                print(f" - {file}")
                return os.path.join(SRPMS_DIR, file)
    except Exception as e:
        print(f"Error listing SRPMS directory: {e}")
    return None

def submit_to_copr(srpm_path):
    """Submit the SRPM to COPR for building"""
    copr_project = "51ddh4r7h/zen-browser"

    # Strip "Wrote: " prefix if present
    if srpm_path.startswith("Wrote: "):
        srpm_path = srpm_path.replace("Wrote: ", "")

    print(f"Submitting {srpm_path} to COPR project {copr_project}...")

    result = subprocess.run(
        ["copr-cli", "build", copr_project, srpm_path],
        capture_output=True,
        text=True
    )

    if result.returncode != 0:
        print(f"Error submitting to COPR: {result.stderr}")
        sys.exit(1)

    print(f"Successfully submitted to COPR: {result.stdout}")

    # Extract the build ID from the output
    build_id_match = re.search(r"Created builds: (\d+)", result.stdout)
    if build_id_match:
        build_id = build_id_match.group(1)
        print(f"Build ID: {build_id}")
        print(f"Build status URL: https://copr.fedorainfracloud.org/coprs/build/{build_id}/")

def main():
    print("Checking for new Zen Browser releases...")
    latest_release = get_latest_release()

    # Check if this is a new version
    with open(SPEC_FILE_PATH, "r") as f:
        spec_content = f.read()
        version_match = re.search(r"Version:\s+(.*)", spec_content)
        if not version_match:
            print("Error: Could not find Version in spec file")
            sys.exit(1)
        current_version = version_match.group(1)

    if current_version == latest_release["version"]:
        print(f"Already at the latest version: {current_version}")
        return

    print(f"New version found: {latest_release['version']}")
    print("Downloading source...")
    download_source(latest_release)

    print("Updating spec file...")
    update_spec_file(latest_release)

    print("Building SRPM...")
    srpm_path = build_srpm()

    print("Submitting to COPR...")
    submit_to_copr(srpm_path)

    print("Done!")

if __name__ == "__main__":
    main()
