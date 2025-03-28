# Zen Browser RPM Builder

Automated RPM package builder for [Zen Browser](https://zen-browser.app) - a customizability, design and privacy-focused   Firefox fork. This repository contains the necessary files to automatically build and publish RPM packages for Fedora Linux via COPR.

## Overview

This project is manually checks for new Zen Browser releases and builds RPM packages when updates are available. Packages are published to the [zen-browser COPR repository](https://copr.fedorainfracloud.org/coprs/51ddh4r7h/zen-browser/).

### Key Components

- [`update-zen-browser.sh`](update-zen-browser.sh) - Shell script that:
  - Checks for new Zen Browser releases via GitHub API
  - Downloads source packages
  - Updates the spec file
  - Builds SRPM packages
  - Submits builds to COPR

- [`zen-browser.spec`](zen-browser.spec) - RPM specification file defining how to package Zen Browser

- [`.github/workflows/script.yml`](.github/workflows/script.yml) - GitHub Actions workflow that:
  - Sets up Fedora container environment
  - Executes the update script
  - Commits spec file changes back to the repository
  - Archives build artifacts

## Installation

To install Zen Browser from this repository:

```bash
# Enable the COPR repository
dnf copr enable 51ddh4r7h/zen-browser

# Install Zen Browser
dnf install zen-browser
```
