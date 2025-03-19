Name:           zen-browser
Version:        1.10b
Release:        1%{?dist}
Summary:        Zen Browser – a customizable, privacy-focused Firefox fork
License:        MPL-2.0
URL:            https://zen-browser.app
Source0:        https://github.com/zen-browser/desktop/releases/download/1.10b/zen.linux-x86_64.tar.xz

ExclusiveArch:      x86_64

Recommends:         (plasma-browser-integration if plasma-workspace)
Recommends:         (gnome-browser-connector if gnome-shell)

Requires(post):     gtk-update-icon-cache

# Disable debuginfo package generation
%define debug_package %{nil}

%description
Zen Browser is an open-source fork of Mozilla Firefox focused on privacy,
customizability, and design. This prebuilt binary release provides a ready-to-run
version of Zen Browser.

%prep
%autosetup -n zen

%build
# Prebuilt binary; no build step required.

%install
rm -rf %{buildroot}
# Copy all files from the extracted "zen" folder into /usr/lib/zen-browser
mkdir -p %{buildroot}/usr/lib/zen-browser
cp -a * %{buildroot}/usr/lib/zen-browser/

# Create a launcher script in /usr/bin to run Zen Browser
mkdir -p %{buildroot}/usr/bin
cat > %{buildroot}/usr/bin/zen-browser << 'EOF'
#!/bin/sh
exec /usr/lib/zen-browser/zen "$@"
EOF
chmod +x %{buildroot}/usr/bin/zen-browser

# Create icons directory and copy icon
mkdir -p %{buildroot}/usr/share/icons/hicolor/128x128/apps/
cp browser/chrome/icons/default/default128.png %{buildroot}/usr/share/icons/hicolor/128x128/apps/zen-browser.png

# Create a .desktop entry file
mkdir -p %{buildroot}/usr/share/applications
cat > %{buildroot}/usr/share/applications/zen-browser.desktop << 'EOF'
[Desktop Entry]
Version=1.10b
Name=Zen Browser
Comment=Experience tranquillity while browsing the web without tracking.
GenericName=Web Browser
Exec=zen-browser %U
Icon=zen-browser
Terminal=false
Type=Application
Categories=Network;WebBrowser;
MimeType=text/html;text/xml;application/xhtml+xml;application/xml;application/rss+xml;application/rdf+xml;
StartupNotify=true
StartupWMClass=zen
EOF

%files
%dir /usr/lib/zen-browser
/usr/lib/zen-browser/*
/usr/bin/zen-browser
/usr/share/applications/zen-browser.desktop
/usr/share/icons/hicolor/128x128/apps/zen-browser.png

%changelog
* Wed Mar 19 2025 COPR Build System <copr-build@fedoraproject.org> - 1.10b-1
- Update to 1.10b
