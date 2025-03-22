#!/bin/bash

# Configuration with environment variable support
function get_rpmbuild_path() {
    # First check if RPM_BUILD_ROOT environment variable is set
    if [ -n "$RPM_BUILD_ROOT" ]; then
        echo "$RPM_BUILD_ROOT"
        return
    fi

    # For GitHub Actions running in Fedora container
    if [ -d "/root/rpmbuild" ]; then
        echo "/root/rpmbuild"
        return
    fi

    # Default to user's home directory
    echo "$HOME/rpmbuild"
}

# Set paths based on environment
RPMBUILD_PATH=$(get_rpmbuild_path)
GITHUB_API_URL="https://api.github.com/repos/zen-browser/desktop/releases/latest"
SPEC_FILE_PATH="$RPMBUILD_PATH/SPECS/zen-browser.spec"
SOURCES_DIR="$RPMBUILD_PATH/SOURCES"
SRPMS_DIR="$RPMBUILD_PATH/SRPMS"

function get_latest_release() {
    echo "Fetching latest release info from GitHub API..."

    # Get release data from GitHub API
    RESPONSE=$(curl -s "$GITHUB_API_URL")
    if [ $? -ne 0 ]; then
        echo "Error accessing GitHub API"
        exit 1
    fi

    # Extract version (tag_name)
    VERSION=$(echo "$RESPONSE" | grep -o '"tag_name":"[^"]*"' | cut -d'"' -f4)

    # Skip twilight/nightly builds (containing 't' in version)
    if [[ "$VERSION" == *t* ]]; then
        echo "Skipping twilight/nightly build version: $VERSION"
        exit 0
    fi

    # Find the Linux x86_64 asset
    DOWNLOAD_URL=$(echo "$RESPONSE" | grep -o '"browser_download_url":"[^"]*linux-x86_64.tar.xz[^"]*"' | cut -d'"' -f4)

    if [ -z "$DOWNLOAD_URL" ]; then
        echo "Could not find Linux x86_64 asset in the release"
        exit 1
    fi

    FILENAME="zen.linux-x86_64.tar.xz"
    PUBLISHED_AT=$(echo "$RESPONSE" | grep -o '"published_at":"[^"]*"' | cut -d'"' -f4)

    echo "VERSION=$VERSION"
    echo "DOWNLOAD_URL=$DOWNLOAD_URL"
    echo "FILENAME=$FILENAME"
    echo "PUBLISHED_AT=$PUBLISHED_AT"
}

function update_spec_file() {
    echo "Updating spec file with new version: $VERSION"

    # Create a temporary file
    TEMP_FILE=$(mktemp)

    # Update main version
    sed -e "s/Version:[ \t]*.*/Version:        $VERSION/" \
        -e "s|Source0:[ \t]*.*|Source0:        https://github.com/zen-browser/desktop/releases/download/$VERSION/zen.linux-x86_64.tar.xz|" \
        "$SPEC_FILE_PATH" > "$TEMP_FILE"

    # Update desktop entry version
    sed -i -e '/\[Desktop Entry\]/,/^$/s/Version=.*/Version='$VERSION'/' "$TEMP_FILE"

    # Add new changelog entry
    TODAY=$(date "+%a %b %d %Y")
    CHANGELOG_ENTRY="%changelog\n* $TODAY COPR Build System <copr-build@fedoraproject.org> - $VERSION-1\n- Update to $VERSION\n"

    # Replace existing changelog with new entry
    sed -i -e '/^%changelog/,$d' "$TEMP_FILE"
    echo -e "$CHANGELOG_ENTRY" >> "$TEMP_FILE"

    # Move the temporary file to the spec file
    mv "$TEMP_FILE" "$SPEC_FILE_PATH"
}

function download_source() {
    echo "Downloading source tarball..."

    # Ensure the SOURCES directory exists
    mkdir -p "$SOURCES_DIR"

    SOURCE_PATH="$SOURCES_DIR/$FILENAME"

    # Download the source
    curl -L -o "$SOURCE_PATH" "$DOWNLOAD_URL"

    if [ $? -ne 0 ]; then
        echo "Error downloading source"
        exit 1
    fi

    echo "Downloaded source to $SOURCE_PATH"
}

function build_srpm() {
    echo "Building SRPM package..."

    # Build SRPM
    RPMBUILD_OUTPUT=$(rpmbuild -bs "$SPEC_FILE_PATH" 2>&1)

    if [ $? -ne 0 ]; then
        echo "Error building SRPM: $RPMBUILD_OUTPUT"
        exit 1
    fi

    # Extract SRPM path from output
    SRPM_PATH=$(echo "$RPMBUILD_OUTPUT" | grep -o "Wrote:.*\.src\.rpm" | cut -d' ' -f2)

    if [ -z "$SRPM_PATH" ]; then
        # Try to find SRPM based on spec file version info
        if [ -f "$SPEC_FILE_PATH" ]; then
            SPEC_VERSION=$(grep "Version:" "$SPEC_FILE_PATH" | awk '{print $2}')
            SPEC_RELEASE=$(grep "Release:" "$SPEC_FILE_PATH" | awk '{print $2}' | sed 's/%{?dist}/.fc41/')
            EXPECTED_PATH="$SRPMS_DIR/zen-browser-$SPEC_VERSION-$SPEC_RELEASE.src.rpm"

            if [ -f "$EXPECTED_PATH" ]; then
                SRPM_PATH="$EXPECTED_PATH"
            fi
        fi
    fi

    if [ -z "$SRPM_PATH" ]; then
        # Find most recent SRPM in SRPMS directory
        mkdir -p "$SRPMS_DIR"
        SRPM_PATH=$(find "$SRPMS_DIR" -name "*.src.rpm" -type f -print -quit)
    fi

    if [ -z "$SRPM_PATH" ]; then
        echo "Could not find built SRPM path"
        echo "rpmbuild output: $RPMBUILD_OUTPUT"
        exit 1
    fi

    echo "Found SRPM: $SRPM_PATH"
    echo "$SRPM_PATH"
}

function submit_to_copr() {
    local srpm_path="$1"
    echo "Submitting $srpm_path to COPR for building..."

    COPR_PROJECT="51ddh4r7h/zen-browser"

    # Submit to COPR
    COPR_OUTPUT=$(copr-cli build "$COPR_PROJECT" "$srpm_path" 2>&1)

    if [ $? -ne 0 ]; then
        echo "Error submitting to COPR: $COPR_OUTPUT"
        exit 1
    fi

    echo "Successfully submitted to COPR: $COPR_OUTPUT"

    # Extract build ID from output
    BUILD_ID=$(echo "$COPR_OUTPUT" | grep -o "Created builds: [0-9]*" | cut -d' ' -f3)

    if [ -n "$BUILD_ID" ]; then
        echo "Build ID: $BUILD_ID"
        echo "Build status URL: https://copr.fedorainfracloud.org/coprs/build/$BUILD_ID/"
    fi
}

function main() {
    echo "Checking for new Zen Browser releases..."

    # Get latest release info
    get_latest_release

    # Check if this is a new version
    if [ -f "$SPEC_FILE_PATH" ]; then
        CURRENT_VERSION=$(grep "Version:" "$SPEC_FILE_PATH" | awk '{print $2}')

        if [ "$CURRENT_VERSION" = "$VERSION" ]; then
            echo "Already at the latest version: $CURRENT_VERSION"
            exit 0
        fi
    else
        echo "Error: Spec file not found at $SPEC_FILE_PATH"
        exit 1
    fi

    echo "New version found: $VERSION"

    # Download source
    download_source

    # Update spec file
    update_spec_file

    # Build SRPM
    SRPM_PATH=$(build_srpm)

    # Submit to COPR
    submit_to_copr "$SRPM_PATH"

    echo "Done!"
}

# Run the main function
main
