# Zen Browser RPM Builder

Automated RPM package builder for [Zen Browser](https://zen-browser.app) - a Firefox fork focused on customizability, design, and privacy. This repository builds and publishes RPM packages for Fedora Linux via COPR.

## Installation

```bash
# Enable COPR repository
dnf copr enable 51ddh4r7h/zen-browser

# Install Zen Browser
dnf install zen-browser
```

## Components

- `update-zen-browser.go` - Go script that checks for new releases, builds and submits packages to COPR
- `zen-browser.spec` - RPM specification file
- GitHub Actions workflow for automated builds

[→ COPR Repository](https://copr.fedorainfracloud.org/coprs/51ddh4r7h/zen-browser/)
