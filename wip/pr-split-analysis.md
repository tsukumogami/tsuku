# PR Split Analysis: `docs/system-lib-backfill`

Generated: 2026-03-01

## Summary

**1,562 changed recipe files** across 4 categories. The branch contains three
distinct types of changes that can be split along clean boundaries.

## Category Breakdown

| Category | Count | Description |
|----------|-------|-------------|
| New tool recipes | 1,372 | New tools (homebrew, cargo, gem installs) |
| New library recipes | 140 | New `type = "library"` recipes |
| Modified library recipes | 23 | Existing libraries: added `[metadata.satisfies]` |
| Modified tool recipes | 27 | Existing tools: added `source = "crates_io"` or `runtime_dependencies` |
| **Total** | **1,562** | |

## Change Types (What Actually Changed)

### 1. Crates.io version source backfill (102 recipes)

22 modified + 80 new recipes using `source = "crates_io"`. The modified tools
already existed on main and only gained `source = "crates_io"` in their
`[version]` section. The 80 new ones are `cargo_install` recipes.

### 2. RubyGems recipes (83 recipes)

83 new recipes using `source = "rubygems"` and `action = "gem_install"`. All
are brand new, none existed on main.

### 3. Homebrew recipes with dependencies (1,349 new + 28 modified)

The bulk of the PR. Most new recipes use `action = "homebrew"` with
`source = "homebrew"` for versioning.

### 4. Library `[metadata.satisfies]` backfill (23 modified libraries)

Existing library recipes gained `[metadata.satisfies]` sections mapping them
to Homebrew formula names. Small, mechanical changes.

### 5. Runtime dependencies backfill (5 modified tool recipes)

fontconfig, git, pngcrush, spatialite, and sqlite gained `runtime_dependencies`
fields pointing to libraries that already exist on main.

## Version Source Distribution

| Source | Count |
|--------|-------|
| homebrew | 1,247 |
| crates_io | 102 |
| rubygems | 83 |
| manual | 1 |
| none/empty | 129 |

## Dependency Analysis

### Overview

| Metric | Count |
|--------|-------|
| Recipes with `runtime_dependencies` | 621 |
| Recipes depending on NEW recipes in this PR | 432 |
| Recipes with NO new deps in this PR | 1,130 |
| New libraries depended upon by other new recipes | 31 |
| New "tools" (type="") depended upon by other new recipes | 213 |

### Dependency Layers

Recipes form a 6-layer DAG based on inter-PR dependencies:

| Layer | Count | Description |
|-------|-------|-------------|
| 0 | 1,081 | No deps on other new recipes (independent) |
| 1 | 303 | Depends only on layer-0 recipes |
| 2 | 100 | Depends on layer-0 and layer-1 |
| 3 | 20 | Depends on layers 0-2 |
| 4 | 4 | Depends on layers 0-3 |
| 5 | 4 | Depends on layers 0-4 (deepest) |

### Most Depended-On New Recipes (Top 20)

These are critical path items -- they must land before their dependents.

| Recipe | Typed as | Dependents in PR |
|--------|----------|-----------------|
| glib | library | 85 |
| qtbase | (empty) | 45 |
| libtiff | (empty) | 25 |
| libusb | library | 25 |
| ffmpeg | (empty) | 23 |
| lz4 | (empty) | 20 |
| gnutls | (empty) | 20 |
| libomp | library | 16 |
| qtdeclarative | (empty) | 16 |
| libogg | library | 15 |
| libgcrypt | (empty) | 15 |
| mpfr | library | 12 |
| libgpg-error | (empty) | 12 |
| protobuf | (empty) | 11 |
| libzip | (empty) | 11 |
| gsl | (empty) | 9 |
| libsndfile | (empty) | 9 |
| libidn2 | library | 9 |
| json-glib | (empty) | 9 |
| libplist | (empty) | 9 |

Note: 213 new "tool" recipes (type="") function as de-facto libraries -- they
are depended upon by other recipes but lack `type = "library"`. This is a data
quality issue but does not affect the split strategy.

### Inter-Library Dependencies (New Libraries)

9 new library recipes depend on other new library recipes:

| Library | Depends on (new library) |
|---------|------------------------|
| graphene | glib |
| gupnp-av | glib |
| jsonrpc-glib | glib |
| libdex | glib |
| libgee | glib |
| libgit2-glib | glib |
| liblqr | glib |
| libslirp | glib |
| unbound | libevent |

All point to **glib** (8 dependents) or **libevent** (1 dependent). Both must
land before their dependent libraries.

### Deepest Dependency Chains (Layer 5)

These 4 recipes have the longest dependency chains in the PR:

**audacious** (layer 5):
```
audacious -> qtmultimedia -> qtquick3d -> qtdeclarative -> qtsvg -> qtbase -> glib
                                       -> qtquicktimeline -> qtdeclarative -> ...
                                       -> qtshadertools -> qtbase -> ...
           -> fluid-synth -> glib, libsndfile -> lame, libogg, mpg123, opus
           -> libsidplayfp -> libgcrypt -> libgpg-error
```

**pc6001vx** (layer 5):
```
pc6001vx -> qtmultimedia -> (same Qt chain as above)
         -> ffmpeg -> dav1d, lame, libvpx, opus, svt-av1, x264, x265
```

**pyqt** (layer 5):
```
pyqt -> qtmultimedia, qtcharts, qtdatavis3d, qtdeclarative, qtquick3d,
        qtscxml, qtsensors, qtserialport, qttools, qtwebchannel, qtwebsockets
     -> (all funnel through qtbase -> glib)
```

**qmmp** (layer 5):
```
qmmp -> qtmultimedia -> (same Qt chain)
     -> ffmpeg -> (same ffmpeg chain)
     -> mplayer -> libcaca -> imlib2 -> libtiff
```

### Qt Dependency Subgraph

The Qt ecosystem forms a significant dependency cluster:

```
Layer 0:  glib, dbus, double-conversion, libb2, md4c
  |
Layer 1:  qtbase (45 dependents)
  |
Layer 2:  qtsvg, qtshadertools, qtconnectivity, qtnetworkauth, qtserialport,
          qtwebsockets, qtsensors, qtremoteobjects
  |
Layer 2:  qtdeclarative (depends on qtbase + qtsvg)
  |
Layer 3:  qttools, qtscxml, qtcharts, qtdatavis3d, qtwebchannel,
          qtquicktimeline, qtquick3d
  |
Layer 4:  qtmultimedia (depends on qtbase + qtdeclarative + qtquick3d + qtshadertools)
  |
Layer 5:  audacious, pc6001vx, pyqt, qmmp
```

## Recipes With No New Dependencies (Safe for Any Batch)

**1,130 recipes** have no dependencies on other new recipes in this PR. These
can be safely included in any batch in any order.

Breakdown of independent recipes:
- 972 new tools
- 109 new libraries
- 49 modified recipes (all 50 modified except spatialite which depends on new `librttopo` and `minizip`)

## Proposed Split Strategy

### PR 1: Modified recipes only (50 files)

- 23 modified libraries (satisfies backfill)
- 22 modified tools (crates_io source backfill)
- 5 modified tools (runtime_dependencies backfill)

Zero dependency risk. These only change existing recipes. Small, reviewable.

### PR 2: Crates.io new recipes (80 files)

- 80 new `cargo_install` recipes with `source = "crates_io"`
- All independent (no `runtime_dependencies`)
- Self-contained: cargo installs don't depend on system libraries

### PR 3: RubyGems new recipes (83 files)

- 83 new `gem_install` recipes with `source = "rubygems"`
- All independent (no `runtime_dependencies`)
- Self-contained: gem installs don't depend on system libraries

### PR 4: Foundation libraries (Layer 0 libraries + high-dep tools) (~280 files)

All 140 new libraries + the ~140 most-depended-on "tool" recipes that function
as libraries (type=""). These are layer 0 of the dependency graph.

Key items: glib, qtbase, ffmpeg, gnutls, libgcrypt, protobuf, libtiff, lz4, etc.

### PR 5+: Remaining homebrew recipes (~1,069 files)

The remaining layer-1+ homebrew recipes. Can be further split into batches
of ~300-400 files alphabetically, since their only dependencies are on
libraries from PR 4.

Split options:
- **By alphabet**: a-e, f-l, m-r, s-z (~270 each)
- **By layer**: layer 1 first, then layer 2+
- **By dependency count**: least-depended first (safest)

## Risk Assessment

| Risk | Impact | Mitigation |
|------|--------|------------|
| Layer 0 dep missing from PR 4 | Dependent recipes fail CI | Run `tsuku install` test for layer-1 samples |
| Type="" misclassification | Library recipes not found by resolver | Flag for follow-up type-field cleanup |
| Qt chain ordering | 4 layer-5 recipes fail | Include full Qt subgraph in single PR |
| Spatialite deps on new librttopo/minizip | Modified recipe fails | Include these 2 new libs in PR 1 or PR 4 |

## Raw Data

### Modified Libraries (23)

abseil, bdw-gc, brotli, cairo, cuda-runtime, expat, geos, gettext, giflib,
gmp, jpeg-turbo, libgit2, libnghttp3, libngtcp2, libpng, libssh2, libxml2,
mesa-vulkan-drivers, pcre2, proj, readline, vulkan-loader, zstd

### Modified Tools (27)

cargo-expand, cargo-hack, cargo-llvm-cov, cargo-nextest, cargo-release,
cargo-sweep, diskus, dotenv-linter, dotslash, espflash, evtx, fontconfig,
gifski, git, killport, komac, maturin, mdbook, oxipng, pfetch, pngcrush,
probe-rs-tools, py-spy, resvg, spatialite, sqlite, try-rs

### New Libraries (140)

ada-url, argtable3, asio, c-ares, cjson, czmq, dav1d, dbus, docbook,
docbook-xsl, double-conversion, eigen, elfutils, exiftool, faiss, fftw,
flac, flux, fmt, folly, fox, freetype, gdbm, glib, gnu-sed, gnuplot,
googletest, graphene, grpc, gstreamer, gstreamer-plugins-base, gupnp-av,
harfbuzz, highway, http-parser, icu4c, imath, isl, jansson, jbig2dec,
jsonrpc-glib, jsoncpp, krb5-config, lapack, leptonica, libarchive,
libass, libb2, libcbor, libdex, libdrm, libedit, libevent, libffi,
libgee, libgit2-glib, libgraphqlparser, libid3tag, libidn, libidn2,
liblinear, liblqr, libmagic, libmicrohttpd, libmpc, libnice,
libogg, libomp, libpq, libraw, libsamplerate, libslirp, libsodium,
libsoxr, libtool, libunistring, libusb, libuv, libvorbis, libxslt,
libzip-dev, lua, luajit, minizip-ng, mpfr, msgpack, nanopb, ncurses,
nlohmann-json, oniguruma, openblas, opencv, openexr, openssl,
open-jtalk, opus, orc, p11-kit, pango, pcaudiolib, pixman,
portaudio, protobuf-dev, pugixml, qt5-base, qtsvg, re2, readline-dev,
sdl2, snappy-dev, spdlog, speexdsp, swig, thrift, tinyxml2,
unbound, vips, vtk, webp, wolfssl, xapian, xerces-c, xmlto, yaml-cpp,
zeromq, zlib, zlib-ng-compat

### Full Dependency Graph (edges between new recipes only)

```
a2ps -> bdw-gc, libpaper
aarch64-elf-gcc -> aarch64-elf-binutils, gmp, isl, mpfr, zstd [NOTE: zstd is MODIFIED, not new]
aarch64-elf-gdb -> gmp, mpfr, ncurses, readline, xz, zstd
abook -> readline, gettext [NOTE: gettext is MODIFIED]
aerc -> notmuch
afflib -> openssl
aircrack-ng -> openssl, pcre2, sqlite
allureofthestars -> gmp, sdl2
aom -> jpeg-xl, libvmaf
apt -> bzip2, gcc-libs, lz4, openssl, perl, xxhash, xz, zlib-ng-compat, zstd
argyll-cms -> jpeg-turbo, libpng, libtiff, openssl
aribb24 -> libpng
arm-linux-gnueabihf-binutils -> zstd
arm-none-eabi-binutils -> zstd
arm-none-eabi-gcc -> arm-none-eabi-binutils, gmp, isl, mpfr, zstd
arm-none-eabi-gdb -> gmp, mpfr, ncurses, readline, xz, zstd
astrometry-net -> cairo, cfitsio, gsl, jpeg-turbo, libpng, numpy, wcslib
astroterm -> argtable3
asymptote -> bdw-gc, gsl, readline
audacious -> faad2, ffmpeg, fluid-synth, glib, lame, libcue, libnotify, libogg, libopenmpt, libsamplerate, libsidplayfp, libsndfile, mpg123, neon, qtbase, qtmultimedia, qtsvg, wavpack
bettercap -> libusb
bitcoin -> berkeley-db
bloaty -> capnp, protobuf, re2
botan -> sqlite
brpc -> gflags, glib, leveldb, openssl, protobuf
bsdiff -> bzip2
bsips -> blake3
btop -> coreutils
cadaver -> neon, readline
calc -> readline
calibre -> chmlib, djvulibre, glib, hunspell, hyphen, icu4c, libmtp, libpng, libstemmer, libtiff, libusb, libwebp, libwmf, little-cms2, mathjax, optipng, podofo, poppler, qtbase, qtimageformats, qtsvg, qtwebengine, speechd, unrar
capstone-emulator -> capstone
castxml -> llvm
cataclysm-dda -> lz4, sdl2, sdl2-image, sdl2-mixer, sdl2-ttf
cd-hit -> libomp
cdk -> ncurses
certbot -> openssl
chocolate-doom -> fluid-synth, libsamplerate, sdl2-net
chrony -> gnutls, nettle
citus -> icu4c, libpq, lz4, openssl, readline, zstd
claws-mail -> glib, gnutls, nettle
clhep -> xerces-c
clingo -> lua
clog -> cmake
conan -> openssl
cracklib-check -> cracklib
cue -> coreutils
damask-grid -> hdf5-mpi, libomp, metis, open-mpi
darcs -> gmp
darkice -> faac, fdk-aac, jack, lame, libogg, libvorbis, opus, two-lame
dateutils -> groff
dcfldd -> openssl
dfu-programmer -> libusb
dfu-util -> libusb
dgen-sdl -> sdl2
dialog -> ncurses
distcc -> binutils, lz4
docbook2x -> docbook, docbook-xsl, libxml2, libxslt, openssl
dosbox-staging -> fluid-synth, glib, iir1, libslirp, sdl2-net, speexdsp
dosbox-x -> fluid-synth, glib, libslirp
dps8m -> libuv
dua-cli -> coreutils
dump1090-mutability -> librtlsdr
dvdbackup -> libdvdread
ecflow-ui -> qtbase, qtcharts, qtsvg
editorconfig-checker -> editorconfig
eiffelstudio -> gtk+3, libpng
eralchemy -> graphviz
erlang -> openssl, unixodbc, wxwidgets
erlang-at26 -> openssl, unixodbc, wxwidgets
etcd-cpp-apiv3 -> cpprestsdk, etcd, gflags, grpc, protobuf
exploitdb -> openssl
exw3 -> wxwidgets
faudio -> gstreamer, sdl2
fbthrift -> fizz, fmt, folly, gflags, glog, mvfst, sodium, wangle, zstd
fceux -> sdl2
feh -> imlib2, libexif, libpng, libtiff, libxpm
ffmpeg -> dav1d, lame, libvpx, opus, svt-av1, x264, x265
ffmpeg-full -> aom, aribb24, dav1d, frei0r, gnutls, jpeg-xl, lame, libbluray, libogg, librist, libsamplerate, libssh, libvidstab, libvmaf, libvpx, llama.cpp, opencore-amr, opus, rav1e, snappy, speex, srt, svt-av1, whisper-cpp, x264, x265, xvid, zimg
ffmpegthumbnailer -> ffmpeg, glib
ffms2 -> ffmpeg
fftw-mpi -> fftw, open-mpi
fig2dev -> fig2dev, libpng
fio -> libaio
flamebearer -> coreutils
fluid-synth -> glib, libsndfile, portaudio
fonttools -> brotli
freeglut -> libxi, mesa
freeimage -> libjpeg, libpng, libtiff, openexr
freerdp -> ffmpeg, openssl, libusb
frotz -> ncurses
fwupd -> glib, gnutls, json-glib, libcbor, libusb, libxmlb, protobuf-c
gammaray -> qtbase, qtconnectivity, qtdeclarative, qtscxml, qtsvg, qttools, qtwebchannel
gbdfed -> gtk+3
gdal -> geos, giflib, hdf5, json-c, libgeotiff, libpng, libpq, libspatialite, libtiff, libxml2, lz4, netcdf, openexr, openjpeg, pcre2, proj, sqlite, xerces-c, xz, zstd
gdb -> gmp, mpfr, ncurses, python, readline, xz, zstd
geeqie -> djvulibre, exiv2, ffmpegthumbnailer, glib, imath, jpeg-xl, libheif, libtiff, little-cms2
gerbil-scheme -> glib, leveldb, lmdb, openssl, sqlite
ghex -> glib
glade -> glib
gleam -> erlang, openssl
glfw -> mesa
gmic -> fftw, glib, libomp, libpng, libtiff, openexr
gnupg -> gnutls, libassuan, libgcrypt, libgpg-error, libusb, npth, pinentry
gnuplot -> glib, libcerf, libtiff, lua, pango, qt5-base, readline
gnuradio -> gsl, log4cpp, numpy, qt5-base, qwt-qt5, sdl2, uhd, volk, zeromq
goaccess -> libmaxminddb, ncurses, openssl
gobuster -> openssl
gpac -> ffmpeg, jpeg-xl, libvpx, little-cms2, openssl, sdl2
gpsd -> libusb, ncurses, pps-tools
gptfdisk -> icu4c, ncurses, popt
grpc -> abseil, c-ares, openssl, protobuf, re2
gstreamer -> glib, orc
gstreamer-plugins-base -> glib, gstreamer, libogg, opus, pango
gti -> coreutils
guile -> bdw-gc, glib, gmp, libffi, libtool, libunistring, readline
gupnp -> glib, libsoup
gupnp-igd -> glib
h2o -> libuv, openssl
hackrf -> fftw, libusb
hdf5 -> libaec
hdf5-mpi -> libaec, open-mpi
hercules-sdl4 -> hercules, sdl2
hiredis -> openssl
htop -> ncurses
httpstat -> coreutils
hunspell-dictionaries -> hunspell
hydra -> libssh, openssl, pcre2
i686-elf-gcc -> gmp, i686-elf-binutils, isl, mpfr, zstd
idevicerestore -> libimobiledevice, libplist, libusb, libusbmuxd
ideviceinstaller -> libimobiledevice, libplist, libzip
imagemagick -> glib, imath, jpeg-xl, libheif, liblqr, libomp, libtiff, libzip, little-cms2
imagemagick-full -> glib, imath, jpeg-xl, libheif, liblqr, libomp, libtiff, libultrahdr, libzip, little-cms2
ios-deploy -> libimobiledevice, libplist, libusbmuxd
ios-webkit-debug-proxy -> libimobiledevice, libplist, libusbmuxd
ispc -> llvm
jmeter -> openjdk
julius -> libsndfile, portaudio
kapacitor -> influxdb
keepassxc -> libgcrypt, libsodium, minizip
keychain -> gnupg
kmod -> lz4, openssl, xz, zstd
kopia -> openssl
kpartx -> json-c, lz4, readline
kpcli -> gnupg
kube-rs -> openssl
laszip -> libtiff
ldapvi -> glib, openldap
ldc -> lz4
ledger -> gmp, mpfr
lftp -> openssl, readline
libbluray -> fontconfig
libcaca -> imlib2
libcdio-paranoia -> libcdio
libepoxy -> mesa
libewf -> openssl
libfido2 -> libcbor, openssl
libgeotiff -> libtiff, proj
libgsf -> glib
libheif -> aom, libde265, libtiff, shared-mime-info, x265
libimobiledevice -> glib, gnutls, libplist, libusbmuxd
libimobiledevice-glue -> libplist
liblouis -> help2man
libnice -> glib, gstreamer
libnotify -> glib
libopenmpt -> libogg, libsndfile, mpg123, portaudio
libpulsar -> protobuf, snappy
libqalculate -> gmp, gnuplot, icu4c, mpfr, readline
libquicktime -> ffmpeg, glib, lame, libdv, libjpeg, libpng, libtiff, libvorbis, schroedinger
librasterlite2 -> libgeotiff, librttopo, libspatialite, libtiff, libxml2, lz4, minizip, proj, sqlite, xz
librttopo -> geos, gmp
libsecret -> glib
libsidplayfp -> libgcrypt
libsndfile -> lame, libogg, mpg123, opus
libsoup -> glib, glib-networking
libspatialite -> geos, librttopo, libxml2, minizip, proj, sqlite
libssh -> openssl
libspiro -> libpaper
libusbmuxd -> libplist
libvidstab -> libomp
libwebp -> giflib, jpeg-turbo, libpng, libtiff
libwmf -> freetype, glib, libpng
libxmlb -> glib, xz
libxmlsec1 -> gnutls, libgcrypt, libxml2, libxslt, nss, openssl
litani -> ninja
llama.cpp -> libomp
luarocks -> lua
lynis -> openssl
macchina -> openssl
mail -> openssl
mailutils -> gnutls, gsasl, ncurses, readline
man-db -> groff, libpipeline
mariadb -> gnutls, groonga, lz4, openssl, pcre2, zstd
mbedtls -> python
mediainfo -> libmediainfo, libzen
mediamtx -> ffmpeg
memcached -> libevent
metabase -> openjdk
minizip -> lz4, xz, zstd
mkvtoolnix -> libvorbis, libogg, pugixml, qt5-base
mlt -> ffmpeg, fftw, glib, jack, libsamplerate, libvorbis, pango, sdl2
mmseqs2 -> libomp
mongosh -> openssl
mpc -> libmpdclient
mpd -> ffmpeg, flac, glib, icu4c, lame, libgcrypt, libid3tag, libnfs, libogg, libsamplerate, libshout, libsndfile, libsoxr, libvorbis, mpg123, opus
mplayer -> libcaca
mpv -> ffmpeg, jpeg-turbo, libarchive, libass, libbluray, libplacebo, little-cms2, lua, mujs, rubberband, uchardet, vapoursynth
mtools -> e2fsprogs
mvfst -> fizz, fmt, folly, gflags, glog, openssl, sodium
nb -> nmap, pandoc, w3m
ncmpcpp -> fftw, libmpdclient, readline, taglib
ncrack -> openssl
net-snmp -> openssl, pcre2
nethack -> ncurses
newsboat -> json-c, ncurses, openssl, sqlite
ngrep -> libpcap
nikto -> openssl
nmap -> liblinear, libpcap, libssh2, lua, openssl, pcre2
nmap-formatter -> nmap
notmuch -> glib, gmime, talloc, xapian
nsd -> libevent, openssl
nss -> nspr
ntopng -> libmaxminddb, libpcap, ndpi, net-snmp, openssl, redis, rrdtool, zeromq
obs-studio -> ffmpeg, jack, librist, mbedtls, pciutils, qt6, rnnoise, srt, x264
ocp -> libogg
offlineimap -> openssl
oha -> openssl
openbabel -> cairo, eigen, rapidjson
opencoarrays -> gcc, open-mpi
openconnect -> gnutls, lz4, stoken
openjdk -> giflib, libpng
openldap -> openssl
openrtsp -> openssl
opensc -> openssl, pcsc-lite
openssh -> libfido2, openldap, openssl
openssl-oqs -> openssl
openvpn -> lz4, openssl, pkcs11-helper
packetbeat -> libpcap
parallel -> perl
pass -> gnupg
pastel-rb -> ruby
pcb -> glib
pcl -> flann, libomp, qhull, vtk
pdal -> gdal, hdf5, libgeotiff, libtiff, lz4, numpy, openssl, proj, zstd, zlib
pdfcrack -> openssl
pdftohtml -> poppler
percona-toolkit -> openssl, perl
pgbouncer -> libevent, openssl
pgcli -> libpq, openssl
pgpool-ii -> libpq, openssl
php -> bzip2, gmp, icu4c, libpng, libsodium, libxml2, libzip, openssl, readline, sqlite, tidy-html5, xz, zlib, zstd
picard -> qtbase, qtdeclarative, qtsvg
pike -> gdbm, gmp, libtiff, nettle, pcre2, sqlite
pioneer -> glew, sdl2, sdl2-image
plplot -> cairo, pango, qtbase, wxwidgets
pmix -> hwloc, libevent
podman -> qemu
podofo -> fontconfig, freetype, libidn, libjpeg, libpng, libtiff, openssl, libxml2
poppler -> cairo, fontconfig, freetype, jpeg-turbo, libtiff, little-cms2, nss, openjpeg
postgis -> geos, json-c, libpq, libtiff, libxml2, pcre2, proj, protobuf-c, sfcgal, sqlite
postgresql -> icu4c, libpq, lz4, openssl, readline, zstd
postfix -> icu4c, openldap, openssl, pcre2
powder -> sdl2
privoxy -> pcre2
proxychains-ng -> openssl
psqlodbc -> libpq, unixodbc
pulumi -> openssl
pure-ftpd -> libsodium, openssl
pyenv -> openssl, readline, sqlite, xz, zlib
python -> openssl, readline, sqlite, xz, zlib
pyqt -> qtbase, qtcharts, qtconnectivity, qtdatavis3d, qtdeclarative, qtmultimedia, qtnetworkauth, qtquick3d, qtremoteobjects, qtscxml, qtsensors, qtserialport, qtshadertools, qtsvg, qttools, qtwebchannel, qtwebsockets
qalculate-qt -> libqalculate, qtbase, qtsvg, qttools
qcachegrind -> qtbase, qtsvg, qttools
qdmr -> libusb, qtbase, qtserialport, qttools, yaml-cpp
qemu -> glib, gnutls, jpeg-turbo, libpng, libslirp, libssh, libusb, lz4, ncurses, pixman, snappy, vde, zstd
qmmp -> faad2, ffmpeg, game-music-emu, glib, jack, libcdio, libcdio-paranoia, libogg, libsamplerate, libsndfile, libxmp, mad, mpg123, mplayer, opus, projectm, qtbase, qtmultimedia, wavpack, wildmidi
qpdf -> gnutls, jpeg-turbo, openssl
qt5-base -> glib
qtcharts -> qtbase, qtdeclarative
qtconnectivity -> qtbase
qtdatavis3d -> qtbase, qtdeclarative
qtdeclarative -> qtbase, qtsvg
qtimageformats -> qtbase
qtmultimedia -> qtbase, qtdeclarative, qtquick3d, qtshadertools
qtnetworkauth -> qtbase
qtquick3d -> assimp, qtbase, qtdeclarative, qtquicktimeline, qtshadertools
qtquick3dphysics -> qtbase, qtdeclarative, qtquick3d, qtshadertools
qtquickeffectmaker -> qtbase, qtdeclarative, qtquick3d, qtshadertools
qtquicktimeline -> qtbase, qtdeclarative
qtremoteobjects -> qtbase, qtdeclarative
qtscxml -> qtbase, qtdeclarative
qtsensors -> qtbase, qtdeclarative
qtserialport -> qtbase
qtshadertools -> qtbase
qttools -> gumbo-parser, qtbase, qtdeclarative
qtwebchannel -> qtbase, qtdeclarative
qtwebsockets -> qtbase, qtdeclarative
rabbitmq -> erlang
rabbitmq-c -> openssl
radare2 -> capstone, lz4, openssl, xxhash
rapidjson -> cmake
rat -> openssl
recutils -> libgcrypt, readline
remind -> tcl-tk
riscv64-elf-gcc -> gmp, isl, mpfr, riscv64-elf-binutils, zstd
riscv64-elf-gdb -> gmp, mpfr, ncurses, readline, xz, zstd
robotfindskitten -> ncurses
robot-framework -> python
rpm -> libarchive, libmagic, lua, openssl, sqlite, zstd
rrdtool -> glib, pango
rubberband -> libsamplerate, libsndfile
samba -> gnutls, krb5, libtasn1, readline, talloc, tdb
sane-backends -> jpeg-turbo, libpng, libtiff, libusb, net-snmp, openssl
saml2aws -> openssl
schroedinger -> glib, liboil, orc
sdcc -> gputils
sdl2-image -> jpeg-xl, libtiff, libwebp, sdl2
sdl2-mixer -> fluid-synth, game-music-emu, libvorbis, mpg123, sdl2
sdl2-net -> sdl2
sdl2-ttf -> sdl2
sfcgal -> cgal, gmp, mpfr
shairport-sync -> libsodium, openssl, popt, soxr
siril -> cfitsio, exiv2, ffmpeg, ffms2, glib, gsl, healpix, jpeg-xl, json-glib, libheif, libomp, libtiff, little-cms2, wcslib, yyjson
sleuthkit -> afflib, libewf, libvhdi, libvmdk, openssl, sqlite, zlib
smlfmt -> gmp
snort -> daq, dnet, hwloc, libpcap, luajit, openssl, pcre2, xz, zlib
sonic-visualiser -> capnp, jack, libsamplerate, libsndfile, portaudio, qtbase, qtsvg, rubberband
sourcekitten -> swift
spek -> ffmpeg, wxwidgets
squid -> gnutls, openssl
srecord -> libgcrypt
ssdeep -> openssl
sslscan -> openssl
stunnel -> openssl
supertux -> sdl2, sdl2-image
swift-protobuf -> swift
synergy-core -> glib, openssl, qtbase
talloc -> python
telegram-cli -> libconfig, lua, openssl, readline
telnet -> ncurses
tesseract -> leptonica, libtiff
the-silver-searcher -> pcre2, xz
theharvester -> openssl
tmate -> libevent, libssh, msgpack, ncurses
tmux -> libevent, ncurses
tn5250 -> ncurses, openssl
tor -> libevent, openssl, xz, zlib, zstd
tox -> libconfig, libsodium, opus
transmission -> libevent, openssl
uhd -> libusb
unix2dos -> openssl
unrar -> openssl
upower -> glib, libgudev, libimobiledevice
urlwatch -> openssl
valgrind -> lz4, openssl
vamp-plugin-sdk -> libsndfile
vapoursynth -> python, zimg
verilator -> python
vim -> lua, ncurses, python, readline, ruby
volk -> cpu-features, orc
w3m -> bdw-gc, ncurses, openssl
wabt -> openssl
watchman -> openssl
wcslib -> cfitsio
weechat -> aspell, gnutls, libgcrypt, lua, ncurses, perl, python, ruby, tcl-tk
wireshark -> glib, gnutls, libgcrypt, libmaxminddb, libnghttp2, libpcap, libssh, libxml2, lua, minizip, opus, snappy
x11vnc -> openssl
xen -> lzo, ncurses, ocaml, python, yajl
xorriso -> readline, xz, zlib
xterm -> ncurses
yara -> openssl, pcre2
yara-x-cli -> yara-x
zeek -> libpcap, openssl
znc -> icu4c, openssl
zoxide -> fzf
zrok -> openssl
zsync -> librsync
zynaddsubfx -> fftw, jack, mxml, portaudio, zlib
```
