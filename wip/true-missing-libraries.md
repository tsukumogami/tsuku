# True Missing Library Dependencies Analysis

Analysis of Homebrew formula dependencies for all tsuku recipes that use
`action = "homebrew"`, cross-referenced against existing recipes and
`metadata.satisfies.homebrew` mappings.

## Summary

- Total tsuku recipes: 1674
- Recipes using `homebrew` action: 1291
- Satisfies mappings defined: 34

### Direct dependencies (from `dependencies` array)

- Unique dep names encountered: 691
- Resolved to existing recipes: 186
- **Truly missing: 505**

### `uses_from_macos` dependencies

- Unique dep names encountered: 39
- Resolved to existing recipes: 16
- Missing: 23

Note: `uses_from_macos` deps are available as system packages on macOS and
most Linux distros. They are lower priority than direct deps.

## False Positives (resolved via satisfies/aliases)

These 2 deps don't match a recipe name directly but resolve
through `metadata.satisfies.homebrew` or Homebrew formula aliases:

| Dep Name | Resolution Path | Recipe |
|---|---|---|
| `gcc` | satisfies | `gcc-libs` |
| `openssl@3` | satisfies | `openssl` |

## Top 50 Missing Direct Dependencies

These are real Homebrew `dependencies` (not `uses_from_macos`), sorted by
how many tsuku recipes need them.

| Rank | Dependency | Dependents | Transitive Deps | Description |
|---|---|---|---|---|
| 1 | `python@3.14` | 36 | 8 | Interpreted, interactive, object-oriented programming langua |
| 2 | `pango` | 35 | 23 | Framework for layout and rendering of i18n text |
| 3 | `gdk-pixbuf` | 34 | 10 | Toolkit for image loading and pixel buffer manipulation |
| 4 | `harfbuzz` | 27 | 19 | OpenType text shaping engine |
| 5 | `libtiff` | 26 | 4 | TIFF library and utilities |
| 6 | `libusb` | 25 | 0 | Library for USB device access |
| 7 | `gtk+3` | 24 | 38 | Toolkit for creating graphical user interfaces |
| 8 | `boost` | 23 | 4 | Collection of portable C++ source libraries |
| 9 | `at-spi2-core` | 22 | 14 | Protocol definitions and daemon for D-Bus at-spi |
| 10 | `lz4` | 21 | 0 | Extremely Fast Compression algorithm |
| 11 | `openjdk` | 18 | 27 | Development kit for the Java programming language |
| 12 | `libvorbis` | 17 | 1 | Vorbis general audio compression codec |
| 13 | `libx11` | 17 | 4 | X.Org: Core X11 protocol client library |
| 14 | `libomp` | 16 | 0 | LLVM's OpenMP runtime library |
| 15 | `libogg` | 15 | 0 | Ogg Bitstream Library |
| 16 | `libarchive` | 14 | 4 | Multi-format archive and compression library |
| 17 | `mpfr` | 13 | 1 | C library for multiple-precision floating-point computations |
| 18 | `fftw` | 13 | 1 | C routines to compute the Discrete Fourier Transform |
| 19 | `webp` | 13 | 7 | Image format providing lossless and lossy compression for we |
| 20 | `adwaita-icon-theme` | 12 | 31 | Icons for the GNOME project |
| 21 | `icu4c@78` | 11 | 0 | C/C++ and Java libraries for Unicode and globalization |
| 22 | `protobuf` | 11 | 1 | Protocol buffers (Google's data interchange format) |
| 23 | `libzip` | 11 | 3 | C library for reading, creating, and modifying zip archives |
| 24 | `ghostscript` | 10 | 39 | Interpreter for PostScript and PDF |
| 25 | `libtool` | 10 | 1 | Generic library support script |
| 26 | `flac` | 9 | 1 | Free lossless audio codec |
| 27 | `libunistring` | 9 | 0 | C string library for manipulating Unicode strings |
| 28 | `gpgme` | 9 | 22 | Library access to GnuPG |
| 29 | `lame` | 8 | 0 | High quality MPEG Audio Layer III (MP3) encoder |
| 30 | `mpg123` | 8 | 0 | MP3 player for Linux and UNIX |
| 31 | `qtsvg` | 8 | 32 | Classes for displaying the contents of SVG files |
| 32 | `libpq` | 8 | 4 | Postgres C API library |
| 33 | `poppler` | 8 | 48 | PDF rendering library (based on the xpdf-3.0 code base) |
| 34 | `fmt` | 7 | 0 | Open-source formatting library for C++ |
| 35 | `libsamplerate` | 7 | 0 | Library for sample rate conversion of audio data |
| 36 | `jansson` | 7 | 0 | C library for encoding, decoding, and manipulating JSON |
| 37 | `libuv` | 7 | 0 | Multi-platform support library with a focus on asynchronous  |
| 38 | `libxext` | 7 | 5 | X.Org: Library for common extensions to the X11 protocol |
| 39 | `pixman` | 7 | 0 | Low-level library for pixel manipulation |
| 40 | `sdl2_mixer` | 7 | 21 | Sample multi-channel audio mixer library |
| 41 | `openblas` | 7 | 9 | Optimized BLAS library |
| 42 | `opus` | 7 | 0 | Audio codec |
| 43 | `minizip` | 7 | 0 | C library for zip/unzip via zLib |
| 44 | `openjpeg` | 7 | 7 | Library for JPEG-2000 image manipulation |
| 45 | `mad` | 6 | 0 | MPEG audio decoder |
| 46 | `libxcb` | 6 | 3 | X.Org: Interface to the X Window System protocol |
| 47 | `sdl2_image` | 6 | 21 | Library for loading images as SDL surfaces and textures |
| 48 | `libao` | 6 | 0 | Cross-platform Audio Library |
| 49 | `node` | 6 | 20 | Open-source, cross-platform JavaScript runtime environment |
| 50 | `nettle` | 6 | 1 | Low-level cryptographic library |

## Top 20 Missing Direct Dependencies (detailed)

### 1. `python@3.14` (36 dependents)

**Description:** Interpreted, interactive, object-oriented programming language
**Transitive deps:** 8
**Keg-only:** False
**Resolved own deps:** `openssl@3` -> `openssl`, `sqlite` -> `sqlite`, `xz` -> `xz`, `zstd` -> `zstd`
**Missing own deps:** `mpdecimal`

**Needed by:**
- `aarch64-elf-gdb`
- `arm-none-eabi-gdb`
- `astrometry-net`
- `bazarr`
- `bento4`
- `cdogs-sdl`
- `chapel`
- `crosstool-ng`
- `distcc`
- `emscripten`
- `fail2ban`
- `fontforge`
- `freeradius-server`
- `gensio`
- `ginac`
- `gnuradio`
- `gplugin`
- `gtk-doc`
- `gupnp`
- `gyb`
- `i386-elf-gdb`
- `knot-resolver`
- `ldns`
- `lensfun`
- `mapserver`
- `notmuch`
- `nwchem`
- `omniorb`
- `partio`
- `polynote`
- ... and 6 more

### 2. `pango` (35 dependents)

**Description:** Framework for layout and rendering of i18n text
**Transitive deps:** 23
**Keg-only:** False
**Resolved own deps:** `cairo` -> `cairo`, `fontconfig` -> `fontconfig`, `freetype` -> `freetype`, `fribidi` -> `fribidi`, `glib` -> `glib`
**Missing own deps:** `harfbuzz`, `libthai`

**Needed by:**
- `claws-mail`
- `dissent`
- `eiffelstudio`
- `enter-tex`
- `ettercap`
- `fontforge`
- `freeciv`
- `gabedit`
- `gedit`
- `geeqie`
- `gerbv`
- `gkrellm`
- `gnome-builder`
- `gnome-papers`
- `gnu-apl`
- `gpredict`
- `gsmartcontrol`
- `gtk-gnutella`
- `gtranslator`
- `gucharmap`
- `gwyddion`
- `homebank`
- `klavaro`
- `libdazzle`
- `mikutter`
- `mlt`
- `nip4`
- `pcb2gcode`
- `pdfpc`
- `pioneers`
- ... and 5 more

### 3. `gdk-pixbuf` (34 dependents)

**Description:** Toolkit for image loading and pixel buffer manipulation
**Transitive deps:** 10
**Keg-only:** False
**Resolved own deps:** `glib` -> `glib`, `jpeg-turbo` -> `jpeg-turbo`, `libpng` -> `libpng`, `gettext` -> `gettext`
**Missing own deps:** `libtiff`

**Needed by:**
- `audacious`
- `claws-mail`
- `dissent`
- `eiffelstudio`
- `enter-tex`
- `ettercap`
- `freeciv`
- `gabedit`
- `gedit`
- `geeqie`
- `gerbv`
- `gkrellm`
- `gnome-papers`
- `gnu-apl`
- `gpredict`
- `gsmartcontrol`
- `gtk-gnutella`
- `gtk-vnc`
- `gupnp-tools`
- `gwyddion`
- `homebank`
- `imagemagick-full`
- `klavaro`
- `libdazzle`
- `mikutter`
- `mlt`
- `nip4`
- `pcb2gcode`
- `pdfpc`
- `pioneers`
- ... and 4 more

### 4. `harfbuzz` (27 dependents)

**Description:** OpenType text shaping engine
**Transitive deps:** 19
**Keg-only:** False
**Resolved own deps:** `cairo` -> `cairo`, `freetype` -> `freetype`, `glib` -> `glib`, `graphite2` -> `graphite2`
**Missing own deps:** `icu4c@78`

**Needed by:**
- `claws-mail`
- `dissent`
- `easyrpg-player`
- `eiffelstudio`
- `ffmpeg-full`
- `freeciv`
- `gabedit`
- `gerbv`
- `gnome-papers`
- `gnu-apl`
- `gpredict`
- `gsmartcontrol`
- `gtk-gnutella`
- `gwyddion`
- `homebank`
- `klavaro`
- `libdazzle`
- `mikutter`
- `mlt`
- `pcb2gcode`
- `pdfpc`
- `pioneers`
- `pqiv`
- `qalculate-gtk`
- `qtbase`
- `spice-gtk`
- `synfig`

### 5. `libtiff` (26 dependents)

**Description:** TIFF library and utilities
**Transitive deps:** 4
**Keg-only:** False
**Resolved own deps:** `jpeg-turbo` -> `jpeg-turbo`, `xz` -> `xz`, `zstd` -> `zstd`
**Status: LEAF - all own deps satisfied, can be created immediately**

**Needed by:**
- `argyll-cms`
- `dcmtk`
- `djvulibre`
- `fontforge`
- `geeqie`
- `gnome-papers`
- `gnuastro`
- `grokj2k`
- `imagemagick-full`
- `imlib2`
- `jbig2enc`
- `leptonica`
- `libgeotiff`
- `libgr`
- `libheif`
- `librasterlite2`
- `little-cms2`
- `openjph`
- `poppler-qt5`
- `povray`
- `proj`
- `sane-backends`
- `siril`
- `spatialite-gui`
- `wxwidgets`
- `xfig`

### 6. `libusb` (25 dependents)

**Description:** Library for USB device access
**Transitive deps:** 0
**Keg-only:** False

**Needed by:**
- `avrdude`
- `ddcutil`
- `dfu-programmer`
- `dfu-util`
- `fwupd`
- `libbladerf`
- `libfreenect`
- `libgphoto2`
- `librealsense`
- `librtlsdr`
- `libusb-compat`
- `limesuite`
- `minipro`
- `open-ocd`
- `openfpgaloader`
- `pcl`
- `qdmr`
- `readsb`
- `rtl-433`
- `sane-backends`
- `spice-gtk`
- `stlink`
- `uhubctl`
- `usbredir`
- `uuu`

### 7. `gtk+3` (24 dependents)

**Description:** Toolkit for creating graphical user interfaces
**Transitive deps:** 38
**Keg-only:** False
**Resolved own deps:** `cairo` -> `cairo`, `fribidi` -> `fribidi`, `glib` -> `glib`, `gettext` -> `gettext`
**Missing own deps:** `at-spi2-core`, `gdk-pixbuf`, `gsettings-desktop-schemas`, `harfbuzz`, `hicolor-icon-theme`, `libepoxy`, `pango`

**Needed by:**
- `claws-mail`
- `eiffelstudio`
- `enter-tex`
- `ettercap`
- `freeciv`
- `gedit`
- `geeqie`
- `gnu-apl`
- `gnuradio`
- `gpredict`
- `gsmartcontrol`
- `gtk-vnc`
- `gucharmap`
- `gupnp-tools`
- `homebank`
- `klavaro`
- `libdazzle`
- `mikutter`
- `pdfpc`
- `pioneers`
- `pqiv`
- `qalculate-gtk`
- `siril`
- `spice-gtk`

### 8. `boost` (23 dependents)

**Description:** Collection of portable C++ source libraries
**Transitive deps:** 4
**Keg-only:** False
**Resolved own deps:** `xz` -> `xz`, `zstd` -> `zstd`
**Missing own deps:** `icu4c@78`

**Needed by:**
- `audiowaveform`
- `avro-cpp`
- `colmap`
- `cryfs`
- `fastnetmon`
- `gnuradio`
- `gource`
- `i2pd`
- `innoextract`
- `logstalgia`
- `ncmpcpp`
- `osm2pgrouting`
- `osmium-tool`
- `pcb2gcode`
- `pcl`
- `pdnsrec`
- `povray`
- `rtabmap`
- `scummvm-tools`
- `supertux`
- `thors-anvil`
- `visp`
- `votca`

### 9. `at-spi2-core` (22 dependents)

**Description:** Protocol definitions and daemon for D-Bus at-spi
**Transitive deps:** 14
**Keg-only:** False
**Resolved own deps:** `glib` -> `glib`, `gettext` -> `gettext`
**Missing own deps:** `dbus`, `libx11`, `libxi`, `libxtst`

**Needed by:**
- `claws-mail`
- `eiffelstudio`
- `ettercap`
- `freeciv`
- `gabedit`
- `gerbv`
- `gnu-apl`
- `gpredict`
- `gsmartcontrol`
- `gtk-gnutella`
- `gucharmap`
- `gwyddion`
- `homebank`
- `klavaro`
- `libdazzle`
- `mikutter`
- `pcb2gcode`
- `pdfpc`
- `pioneers`
- `pqiv`
- `qalculate-gtk`
- `spice-gtk`

### 10. `lz4` (21 dependents)

**Description:** Extremely Fast Compression algorithm
**Transitive deps:** 0
**Keg-only:** False

**Needed by:**
- `apt`
- `colmap`
- `dar`
- `librasterlite2`
- `micromamba`
- `osmcoastline`
- `osmium-tool`
- `pcl`
- `percona-server`
- `percona-xtrabackup`
- `pgbackrest`
- `rizin`
- `rtabmap`
- `sambamba`
- `spatialite-gui`
- `spice-gtk`
- `squashfs`
- `ugrep`
- `visp`
- `wireshark`
- `zstd`

### 11. `openjdk` (18 dependents)

**Description:** Development kit for the Java programming language
**Transitive deps:** 27
**Keg-only:** True
**Resolved own deps:** `freetype` -> `freetype`, `giflib` -> `giflib`, `jpeg-turbo` -> `jpeg-turbo`, `libpng` -> `libpng`, `little-cms2` -> `little-cms2`
**Missing own deps:** `harfbuzz`

**Needed by:**
- `alda`
- `apache-polaris`
- `bbtools`
- `bigloo`
- `cljfmt`
- `emscripten`
- `erlang-language-platform`
- `javacc`
- `jhipster`
- `joern`
- `jruby`
- `jsonschema2pojo`
- `jython`
- `metals`
- `pdftk-java`
- `polynote`
- `prog8`
- `sleuthkit`

### 12. `libvorbis` (17 dependents)

**Description:** Vorbis general audio compression codec
**Transitive deps:** 1
**Keg-only:** False
**Missing own deps:** `libogg`

**Needed by:**
- `audacious`
- `cdrdao`
- `darkice`
- `easyrpg-player`
- `ffmpeg-full`
- `frotz`
- `libopenmpt`
- `libsndfile`
- `mlt`
- `openmsx`
- `qmmp`
- `scummvm`
- `scummvm-tools`
- `sox-ng`
- `supertux`
- `vgmstream`
- `vorbis-tools`

### 13. `libx11` (17 dependents)

**Description:** X.Org: Core X11 protocol client library
**Transitive deps:** 4
**Keg-only:** False
**Missing own deps:** `libxcb`, `xorgproto`

**Needed by:**
- `cairo`
- `ddcutil`
- `eiffelstudio`
- `feh`
- `ffmpeg-full`
- `fricas`
- `gnu-apl`
- `imlib2`
- `libxpm`
- `ngspice`
- `pdfpc`
- `rxvt-unicode`
- `spice-gtk`
- `xeyes`
- `xfig`
- `xorg-server`
- `xsel`

### 14. `libomp` (16 dependents)

**Description:** LLVM's OpenMP runtime library
**Transitive deps:** 0
**Keg-only:** True

**Needed by:**
- `colmap`
- `cp2k`
- `damask-grid`
- `dynare`
- `gromacs`
- `imagemagick-full`
- `mmseqs2`
- `nwchem`
- `par2`
- `pcl`
- `rtabmap`
- `siril`
- `suite-sparse`
- `synfig`
- `visp`
- `votca`

### 15. `libogg` (15 dependents)

**Description:** Ogg Bitstream Library
**Transitive deps:** 0
**Keg-only:** False

**Needed by:**
- `audacious`
- `darkice`
- `easyrpg-player`
- `ffmpeg-full`
- `libopenmpt`
- `libsndfile`
- `openmsx`
- `qmmp`
- `scummvm`
- `scummvm-tools`
- `sox-ng`
- `speex`
- `supertux`
- `vgmstream`
- `vorbis-tools`

### 16. `libarchive` (14 dependents)

**Description:** Multi-format archive and compression library
**Transitive deps:** 4
**Keg-only:** True
**Resolved own deps:** `xz` -> `xz`, `zstd` -> `zstd`
**Missing own deps:** `libb2`, `lz4`

**Needed by:**
- `fceux`
- `ffmpeg-full`
- `fwup`
- `fwupd`
- `geeqie`
- `gnome-papers`
- `lanraragi`
- `lnav`
- `micromamba`
- `pixz`
- `pqiv`
- `qmmp`
- `reprepro`
- `rpm2cpio`

### 17. `mpfr` (13 dependents)

**Description:** C library for multiple-precision floating-point computations
**Transitive deps:** 1
**Keg-only:** False
**Resolved own deps:** `gmp` -> `gmp`
**Status: LEAF - all own deps satisfied, can be created immediately**

**Needed by:**
- `aarch64-elf-gcc`
- `aarch64-elf-gdb`
- `arm-none-eabi-gcc`
- `arm-none-eabi-gdb`
- `colmap`
- `gcc-libs`
- `i386-elf-gdb`
- `i686-elf-gcc`
- `libqalculate`
- `qalculate-qt`
- `riscv64-elf-gcc`
- `riscv64-elf-gdb`
- `suite-sparse`

### 18. `fftw` (13 dependents)

**Description:** C routines to compute the Discrete Fourier Transform
**Transitive deps:** 1
**Keg-only:** False
**Missing own deps:** `libomp`

**Needed by:**
- `asymptote`
- `cp2k`
- `damask-grid`
- `gnuradio`
- `gromacs`
- `gwyddion`
- `inspectrum`
- `mlt`
- `ncmpcpp`
- `ngspice`
- `siril`
- `synfig`
- `votca`

### 19. `webp` (13 dependents)

**Description:** Image format providing lossless and lossy compression for web images
**Transitive deps:** 7
**Keg-only:** False
**Resolved own deps:** `giflib` -> `giflib`, `jpeg-turbo` -> `jpeg-turbo`, `libpng` -> `libpng`
**Missing own deps:** `libtiff`

**Needed by:**
- `ffmpeg-full`
- `geeqie`
- `imagemagick-full`
- `jbig2enc`
- `jp2a`
- `leptonica`
- `libheif`
- `librasterlite2`
- `pixlet`
- `pqiv`
- `spatialite-gui`
- `tronbyt-server`
- `wxwidgets`

### 20. `adwaita-icon-theme` (12 dependents)

**Description:** Icons for the GNOME project
**Transitive deps:** 31
**Keg-only:** False
**Missing own deps:** `librsvg`

**Needed by:**
- `enter-tex`
- `freeciv`
- `gedit`
- `geeqie`
- `gnome-builder`
- `gnome-papers`
- `gnuradio`
- `gpredict`
- `gtranslator`
- `homebank`
- `klavaro`
- `qalculate-gtk`

## Top 20 Missing `uses_from_macos` Dependencies

These are lower priority - they're available as OS packages. But if tsuku
wants full self-containment on Linux, they'd need recipes too.

Note: 12 deps appear in both this list and the direct deps list above (some
formulas list them as direct deps while others list them as `uses_from_macos`).
Those are: flex, krb5, libedit, libffi, libpcap, libxcrypt, libxslt, llvm,
m4, openldap, unzip, vim.

| Rank | Dependency | Dependents | Description |
|---|---|---|---|
| 1 | `python` | 41 |  |
| 2 | `libffi` | 34 | Portable Foreign Function Interface library |
| 3 | `libxcrypt` | 24 | Extended crypt library for descrypt, md5crypt, bcrypt, and o |
| 4 | `flex` | 23 | Fast Lexical Analyzer, generates Scanners (tokenizers) |
| 5 | `libxslt` | 21 | C XSLT library for GNOME |
| 6 | `swift` | 17 | High-performance system programming language |
| 7 | `libpcap` | 16 | Portable library for network traffic capture |
| 8 | `libedit` | 14 | BSD-style licensed readline alternative |
| 9 | `krb5` | 13 | Network authentication protocol |
| 10 | `llvm` | 7 | Next-gen compiler infrastructure |
| 11 | `unzip` | 6 | Extraction utility for .zip compressed archives |
| 12 | `m4` | 6 | Macro processing language |
| 13 | `cyrus-sasl` | 6 | Simple Authentication and Security Layer |
| 14 | `vim` | 4 | Vi 'workalike' with many additional features |
| 15 | `zip` | 3 | Compression and file packaging/archive utility |
| 16 | `ed` | 1 | Classic UNIX line editor |
| 17 | `netcat` | 1 | Utility for managing network connections |
| 18 | `pax` | 1 | Portable Archive Interchange archive tool |
| 19 | `openldap` | 1 | Open source suite of directory software |
| 20 | `pod2man` | 1 | Perl documentation generator |

## Leaf Libraries: Direct Deps That Can Be Created Immediately

These missing direct dependencies have ALL their own Homebrew `dependencies`
already satisfied by existing tsuku recipes. They can be turned into recipes
right now.

**Total leaf libraries (direct deps): 275**

| Rank | Library | Dependents Unblocked | Transitive Deps | Description |
|---|---|---|---|---|
| 1 | `libtiff` | 26 | 4 | TIFF library and utilities |
| 2 | `libusb` | 25 | 0 | Library for USB device access |
| 3 | `lz4` | 21 | 0 | Extremely Fast Compression algorithm |
| 4 | `libomp` | 16 | 0 | LLVM's OpenMP runtime library |
| 5 | `libogg` | 15 | 0 | Ogg Bitstream Library |
| 6 | `mpfr` | 13 | 1 | C library for multiple-precision floating-point co |
| 7 | `icu4c@78` | 11 | 0 | C/C++ and Java libraries for Unicode and globaliza |
| 8 | `protobuf` | 11 | 1 | Protocol buffers (Google's data interchange format |
| 9 | `libzip` | 11 | 3 | C library for reading, creating, and modifying zip |
| 10 | `libunistring` | 9 | 0 | C string library for manipulating Unicode strings |
| 11 | `lame` | 8 | 0 | High quality MPEG Audio Layer III (MP3) encoder |
| 12 | `mpg123` | 8 | 0 | MP3 player for Linux and UNIX |
| 13 | `qtsvg` | 8 | 32 | Classes for displaying the contents of SVG files |
| 14 | `fmt` | 7 | 0 | Open-source formatting library for C++ |
| 15 | `libsamplerate` | 7 | 0 | Library for sample rate conversion of audio data |
| 16 | `jansson` | 7 | 0 | C library for encoding, decoding, and manipulating |
| 17 | `libuv` | 7 | 0 | Multi-platform support library with a focus on asy |
| 18 | `pixman` | 7 | 0 | Low-level library for pixel manipulation |
| 19 | `opus` | 7 | 0 | Audio codec |
| 20 | `minizip` | 7 | 0 | C library for zip/unzip via zLib |
| 21 | `mad` | 6 | 0 | MPEG audio decoder |
| 22 | `libao` | 6 | 0 | Cross-platform Audio Library |
| 23 | `nettle` | 6 | 1 | Low-level cryptographic library |
| 24 | `json-c` | 6 | 0 | JSON parser for C |
| 25 | `coreutils` | 5 | 1 | GNU File, Shell, and Text utilities |
| 26 | `eigen` | 5 | 0 | C++ template library for linear algebra |
| 27 | `openldap` | 5 | 2 | Open source suite of directory software |
| 28 | `libmaxminddb` | 5 | 0 | C library for the MaxMind DB file format |
| 29 | `portaudio` | 5 | 0 | Cross-platform library for audio I/O |
| 30 | `imath` | 5 | 0 | Library of 2D and 3D vector, matrix, and math oper |
| 31 | `libpcap` | 5 | 0 | Portable library for network traffic capture |
| 32 | `xxhash` | 4 | 0 | Extremely fast non-cryptographic hash algorithm |
| 33 | `zlib-ng-compat` | 4 | 0 | Zlib replacement with optimizations for next gener |
| 34 | `libsoxr` | 4 | 0 | High quality, one-dimensional sample-rate conversi |
| 35 | `qtmultimedia` | 4 | 38 | Provides APIs for playing back and recording audio |
| 36 | `lzo` | 4 | 0 | Real-time data compression library |
| 37 | `gflags` | 4 | 0 | Library for processing command-line flags |
| 38 | `llvm@21` | 4 | 3 | Next-gen compiler infrastructure |
| 39 | `hiredis` | 4 | 2 | Minimalistic client for Redis |
| 40 | `yaml-cpp` | 4 | 0 | C++ YAML parser and emitter for YAML 1.2 spec |
| 41 | `libexif` | 4 | 2 | EXIF parsing library |
| 42 | `popt` | 4 | 0 | Library like getopt(3) with a number of enhancemen |
| 43 | `qhull` | 4 | 0 | Computes convex hulls in n dimensions |
| 44 | `librttopo` | 4 | 1 | RT Topology Library |
| 45 | `libtommath` | 4 | 0 | C library for number theoretic multiple-precision  |
| 46 | `isl` | 3 | 1 | Integer Set Library for the polyhedral model |
| 47 | `berkeley-db@5` | 3 | 0 | High performance key/value database |
| 48 | `wcslib` | 3 | 1 | Library and utilities for the FITS World Coordinat |
| 49 | `libmodplug` | 3 | 0 | Library from the Modplug-XMMS project |
| 50 | `wavpack` | 3 | 0 | Hybrid lossless audio compression |
| 51 | `hwloc` | 3 | 0 | Portable abstraction of the hierarchical topology  |
| 52 | `libmagic` | 3 | 0 | Implementation of the file(1) command |
| 53 | `libsodium` | 3 | 0 | NaCl networking and cryptography library |
| 54 | `graphene` | 3 | 4 | Thin layer of graphic data types |
| 55 | `speexdsp` | 3 | 0 | Speex audio processing library |
| 56 | `x264` | 3 | 0 | H.264/AVC encoder |
| 57 | `libvpx` | 3 | 0 | VP8/VP9 video codec |
| 58 | `talloc` | 3 | 0 | Hierarchical, reference-counted memory pool with d |
| 59 | `glfw` | 3 | 0 | Multi-platform library for OpenGL applications |
| 60 | `hicolor-icon-theme` | 3 | 0 | Fallback theme for FreeDesktop.org icon themes |
| 61 | `dbus` | 3 | 0 | Message bus system, providing inter-application co |
| 62 | `miniupnpc` | 3 | 0 | UPnP IGD client library and daemon |
| 63 | `lmdb` | 3 | 0 | Lightning memory-mapped database: key-value data s |
| 64 | `libimobiledevice-glue` | 3 | 1 | Library with common system API code for libimobile |
| 65 | `tinyxml2` | 3 | 0 | Improved tinyxml (in memory efficiency and size) |
| 66 | `net-snmp` | 3 | 2 | Implements SNMP v1, v2c, and v3, using IPv4 and IP |
| 67 | `qtserialport` | 3 | 32 | Provides classes to interact with hardware and vir |
| 68 | `libbs2b` | 2 | 7 | Bauer stereophonic-to-binaural DSP |
| 69 | `leveldb` | 2 | 1 | Key-value storage library with ordered mapping |
| 70 | `utf8proc` | 2 | 0 | Clean C library for processing UTF-8 Unicode data |
| 71 | `llvm@20` | 2 | 3 | Next-gen compiler infrastructure |
| 72 | `sdl2_net` | 2 | 1 | Small sample cross-platform networking library |
| 73 | `metis` | 2 | 0 | Programs that partition graphs and order matrices |
| 74 | `argon2` | 2 | 0 | Password hashing library and CLI utility |
| 75 | `concurrencykit` | 2 | 0 | Aid design and implementation of concurrent system |
| 76 | `libslirp` | 2 | 4 | General purpose TCP-IP emulator |
| 77 | `woff2` | 2 | 1 | Utilities to create and convert Web Open Font File |
| 78 | `libxmp` | 2 | 0 | C library for playback of module music (MOD, S3M,  |
| 79 | `inih` | 2 | 0 | Simple .INI file parser in C |
| 80 | `qtcharts` | 2 | 34 | UI Components for displaying visually pleasing cha |
| 81 | `libgee` | 2 | 4 | Collection library providing GObject-based interfa |
| 82 | `libnet` | 2 | 0 | C library for creating IP packets |
| 83 | `yyjson` | 2 | 0 | High performance JSON library written in ANSI C |
| 84 | `aom` | 2 | 16 | Codec library for encoding and decoding AV1 video  |
| 85 | `frei0r` | 2 | 0 | Minimalistic plugin API for video effects |
| 86 | `libssh` | 2 | 2 | C library SSHv1/SSHv2 client and server protocols |
| 87 | `libvidstab` | 2 | 0 | Transcode video stabilization plugin |
| 88 | `srt` | 2 | 2 | Secure Reliable Transport |
| 89 | `libcbor` | 2 | 0 | CBOR protocol implementation for C and others |
| 90 | `usb.ids` | 2 | 0 | Repository of vendor, device, subsystem and device |
| 91 | `qtconnectivity` | 2 | 32 | Provides access to Bluetooth hardware |
| 92 | `qtscxml` | 2 | 34 | Provides functionality to create state machines fr |
| 93 | `qtwebchannel` | 2 | 34 | Bridges the gap between Qt applications and HTML/J |
| 94 | `mbedtls@3` | 2 | 0 | Cryptographic & SSL/TLS library |
| 95 | `libiconv` | 2 | 0 | Conversion library |
| 96 | `terminal-notifier` | 2 | 0 | Send macOS User Notifications from the command-lin |
| 97 | `libsigc++@2` | 2 | 0 | Callback framework for C++ |
| 98 | `docbook` | 2 | 0 | Standard XML representation system for technical d |
| 99 | `liblqr` | 2 | 4 | C/C++ seam carving library |
| 100 | `cjson` | 2 | 0 | Ultralightweight JSON parser in ANSI C |
| 101 | `libmicrohttpd` | 2 | 13 | Light HTTP/1.1 server library |
| 102 | `libepoxy` | 2 | 0 | Library for handling OpenGL function pointer manag |
| 103 | `game-music-emu` | 2 | 0 | Videogame music file emulator collection |
| 104 | `libev` | 2 | 0 | Asynchronous event library |
| 105 | `tokyo-cabinet` | 2 | 0 | Lightweight database library |
| 106 | `capstone` | 2 | 2 | Multi-platform, multi-architecture disassembly fra |
| 107 | `pandoc` | 2 | 1 | Swiss-army knife of markup format conversion |
| 108 | `nss` | 2 | 1 | Libraries for security-enabled client and server a |
| 109 | `molten-vk` | 2 | 0 | Implementation of the Vulkan graphics and compute  |
| 110 | `libxlsxwriter` | 2 | 0 | C library for creating Excel XLSX files |
| 111 | `libpaper` | 1 | 0 | Library for handling paper characteristics |
| 112 | `argtable3` | 1 | 0 | ANSI C library for parsing GNU-style command-line  |
| 113 | `libcue` | 1 | 0 | Cue sheet parser library for C |
| 114 | `libsidplayfp` | 1 | 4 | Library to play Commodore 64 music |
| 115 | `libid3tag` | 1 | 0 | ID3 tag manipulation library |
| 116 | `libre` | 1 | 2 | Toolkit library for asynchronous network I/O with  |
| 117 | `protobuf@33` | 1 | 1 | Protocol buffers (Google's data interchange format |
| 118 | `jemalloc` | 1 | 0 | Implementation of malloc emphasizing fragmentation |
| 119 | `kapacitor` | 1 | 0 | Open source time series data processor |
| 120 | `libetpan` | 1 | 0 | Portable mail library handling several protocols |
| 121 | `libffcall` | 1 | 0 | GNU Foreign Function Interface library |
| 122 | `libsigsegv` | 1 | 0 | Library for handling page faults in user mode |
| 123 | `lpeg` | 1 | 0 | Parsing Expression Grammars For Lua |
| 124 | `binutils` | 1 | 3 | GNU binary tools for native development |
| 125 | `flex` | 1 | 2 | Fast Lexical Analyzer, generates Scanners (tokeniz |
| 126 | `lzip` | 1 | 0 | LZMA-based compression program similar to gzip or  |
| 127 | `m4` | 1 | 0 | Macro processing language |
| 128 | `grep` | 1 | 1 | GNU grep, egrep and fgrep |
| 129 | `libfuse@2` | 1 | 0 | Reference implementation of the Linux FUSE interfa |
| 130 | `fdk-aac` | 1 | 0 | Standalone library of the Fraunhofer FDK AAC code  |
| 131 | `two-lame` | 1 | 0 | Optimized MPEG Audio Layer 2 (MP2) encoder |
| 132 | `i2c-tools` | 1 | 0 | Heterogeneous set of I2C tools for Linux |
| 133 | `git-delta` | 1 | 5 | Syntax-highlighting pager for git and diff output |
| 134 | `docker-machine` | 1 | 0 | Create Docker hosts locally and on cloud providers |
| 135 | `iir1` | 1 | 0 | DSP IIR realtime filter library written in C++ |
| 136 | `jsoncpp` | 1 | 0 | Library for interacting with JSON |
| 137 | `potrace` | 1 | 0 | Convert bitmaps to vector graphics |
| 138 | `mujs` | 1 | 0 | Embeddable Javascript interpreter |
| 139 | `log4cpp` | 1 | 0 | Configurable logging for C++ |
| 140 | `isa-l` | 1 | 0 | Intelligent Storage Acceleration Library |
| 141 | `libdeflate` | 1 | 0 | Heavily optimized DEFLATE/zlib/gzip compression an |
| 142 | `help2man` | 1 | 5 | Automatically generate simple man pages |
| 143 | `libwapcaplet` | 1 | 0 | String internment library |
| 144 | `tre` | 1 | 0 | Lightweight, POSIX-compliant regular expression (r |
| 145 | `aribb24` | 1 | 1 | Library for ARIB STD-B24, decoding JIS 8 bit chara |
| 146 | `llama.cpp` | 1 | 2 | LLM inference in C/C++ |
| 147 | `opencore-amr` | 1 | 0 | Audio codecs extracted from Android open source pr |
| 148 | `whisper-cpp` | 1 | 1 | Port of OpenAI's Whisper model in C/C++ |
| 149 | `xvid` | 1 | 0 | High-performance, high-quality MPEG-4 video librar |
| 150 | `zimg` | 1 | 0 | Scaling, colorspace conversion, and dithering libr |
| 151 | `libspiro` | 1 | 0 | Library to simplify the drawing of curves |
| 152 | `libuninameslist` | 1 | 0 | Library of Unicode names and annotation data |
| 153 | `jlog` | 1 | 0 | Pure C message queue with subscribers and publishe |
| 154 | `confuse` | 1 | 0 | Configuration file parser library written in C |
| 155 | `gsettings-desktop-schemas` | 1 | 4 | GSettings schemas for desktop components |
| 156 | `cln` | 1 | 1 | Class Library for Numbers |
| 157 | `libsecret` | 1 | 6 | Library for storing/retrieving passwords and other |
| 158 | `editorconfig` | 1 | 1 | Maintain consistent coding style between multiple  |
| 159 | `jsonrpc-glib` | 1 | 5 | GNOME library to communicate with JSON-RPC based p |
| 160 | `libdex` | 1 | 4 | Future-based programming for GLib-based applicatio |
| 161 | `libgit2-glib` | 1 | 8 | Glib wrapper library around libgit2 git access lib |
| 162 | `exempi` | 1 | 0 | Library to parse XMP metadata |
| 163 | `rpds-py` | 1 | 0 | Python bindings to Rust's persistent data structur |
| 164 | `unbound` | 1 | 4 | Validating, recursive, caching DNS resolver |
| 165 | `uchardet` | 1 | 0 | Encoding detector library |
| 166 | `lmfit` | 1 | 0 | C library for Levenberg-Marquardt minimization and |
| 167 | `gupnp-av` | 1 | 4 | Library to help implement UPnP A/V profiles |
| 168 | `certifi` | 1 | 1 | Mozilla CA bundle for Python |
| 169 | `libaec` | 1 | 0 | Adaptive Entropy Coding implementing Golomb-Rice a |
| 170 | `ic-wasm` | 1 | 0 | CLI tool for performing Wasm transformations speci |
| 171 | `libultrahdr` | 1 | 1 | Reference codec for the Ultra HDR format |
| 172 | `libjodycode` | 1 | 0 | Shared code used by several utilities written by J |
| 173 | `highway` | 1 | 0 | Performance-portable, length-agnostic SIMD with ru |
| 174 | `libfixposix` | 1 | 0 | Thin wrapper over POSIX syscalls |
| 175 | `karchive` | 1 | 32 | Reading, creating, and manipulating file archives |
| 176 | `fstrm` | 1 | 3 | Frame Streams implementation in C |
| 177 | `cpanminus` | 1 | 0 | Get, unpack, build, and install modules from CPAN |
| 178 | `abook` | 1 | 3 | Address book with mutt support |
| 179 | `libudfread` | 1 | 0 | Universal Disk Format reader |
| 180 | `libde265` | 1 | 0 | Open h.265 video codec implementation |
| 181 | `shared-mime-info` | 1 | 4 | Database of common MIME types |
| 182 | `libtatsu` | 1 | 1 | Library handling the communication with Apple's Ta |
| 183 | `cunit` | 1 | 0 | Lightweight unit testing framework for C |
| 184 | `osinfo-db` | 1 | 0 | Osinfo database of operating systems for virtualiz |
| 185 | `marisa` | 1 | 0 | Matching Algorithm with Recursively Implemented St |
| 186 | `fltk` | 1 | 2 | Cross-platform C++ GUI toolkit |
| 187 | `go@1.24` | 1 | 0 | Open source programming language to build simple/r |
| 188 | `llvm@19` | 1 | 3 | Next-gen compiler infrastructure |
| 189 | `libdvdcss` | 1 | 0 | Access DVDs as block devices without the decryptio |
| 190 | `gsasl` | 1 | 4 | SASL library command-line interface |
| 191 | `util-macros` | 1 | 0 | X.Org: Set of autoconf macros used to build other  |
| 192 | `xorgproto` | 1 | 0 | X.Org: Protocol Headers |
| 193 | `libpipeline` | 1 | 0 | C library for manipulating pipelines of subprocess |
| 194 | `fcgi` | 1 | 0 | Protocol for interfacing interactive programs with |
| 195 | `libmodbus` | 1 | 0 | Portable modbus library |
| 196 | `dosfstools` | 1 | 0 | Tools to create, check and label file systems of t |
| 197 | `mtools` | 1 | 0 | Tools for manipulating MSDOS files |
| 198 | `reproc` | 1 | 0 | Cross-platform (C99/C++11) process library |
| 199 | `simdjson` | 1 | 0 | SIMD-accelerated C++ JSON parser |
| 200 | `diffutils` | 1 | 0 | File comparison utilities |
| 201 | `nim` | 1 | 0 | Statically typed compiled systems programming lang |
| 202 | `beagle` | 1 | 0 | Evaluate the likelihood of sequence evolution on t |
| 203 | `glpk` | 1 | 1 | Library for Linear and Mixed-Integer Programming |
| 204 | `gcab` | 1 | 4 | Windows installer (.MSI) tool |
| 205 | `libmpdclient` | 1 | 0 | Library for MPD in the C, C++, and Objective-C lan |
| 206 | `msgpack` | 1 | 0 | Library for a binary-based efficient data intercha |
| 207 | `bstring` | 1 | 0 | Fork of Paul Hsieh's Better String Library |
| 208 | `cracklib` | 1 | 2 | LibCrack password checking library |
| 209 | `iniparser` | 1 | 0 | Library for parsing ini files |
| 210 | `libngspice` | 1 | 0 | Spice circuit simulator as shared library |
| 211 | `sfsexp` | 1 | 0 | Small Fast S-Expression Library |
| 212 | `xapian` | 1 | 0 | C++ search engine library |
| 213 | `discount` | 1 | 0 | C implementation of Markdown |
| 214 | `perl-dbd-mysql` | 1 | 9 | MySQL driver for the Perl5 Database Interface (DBI |
| 215 | `libpg_query` | 1 | 0 | C library for accessing the PostgreSQL parser outs |
| 216 | `tass64` | 1 | 0 | Multi pass optimizing macro assembler for the 65xx |
| 217 | `plotutils` | 1 | 1 | C/C++ function library for exporting 2-D vector gr |
| 218 | `qtdatavis3d` | 1 | 34 | Provides functionality for 3D visualization |
| 219 | `qtnetworkauth` | 1 | 32 | Provides support for OAuth-based authorization to  |
| 220 | `qtremoteobjects` | 1 | 34 | Provides APIs for inter-process communication |
| 221 | `qtsensors` | 1 | 34 | Provides access to sensors via QML and C++ interfa |
| 222 | `qtwebsockets` | 1 | 34 | Provides WebSocket communication compliant with RF |
| 223 | `mpdecimal` | 1 | 0 | Library for decimal floating point arithmetic |
| 224 | `pkcs11-helper` | 1 | 2 | Library to simplify the interaction with PKCS#11 |
| 225 | `libcddb` | 1 | 1 | CDDB server access library |
| 226 | `libcdio-paranoia` | 1 | 1 | CD paranoia on top of libcdio |
| 227 | `libmms` | 1 | 4 | Library for parsing mms:// and mmsh:// network str |
| 228 | `mplayer` | 1 | 19 | UNIX movie player |
| 229 | `projectm` | 1 | 1 | Milkdrop-compatible music visualizer |
| 230 | `wildmidi` | 1 | 0 | Simple software midi player |
| 231 | `double-conversion` | 1 | 0 | Binary-decimal and decimal-binary routines for IEE |
| 232 | `libb2` | 1 | 0 | Secure hashing function |
| 233 | `md4c` | 1 | 0 | C Markdown parser. Fast. SAX-like interface |
| 234 | `assimp` | 1 | 0 | Portable library for importing many well-known 3D  |
| 235 | `qtquicktimeline` | 1 | 34 | Enables keyframe-based animations and parameteriza |
| 236 | `gumbo-parser` | 1 | 0 | C99 library for parsing HTML5 |
| 237 | `libptytty` | 1 | 0 | Library for OS-independent pseudo-TTY management |
| 238 | `ocaml` | 1 | 0 | General purpose programming language in the ML fam |
| 239 | `sdl3` | 1 | 0 | Low-level access to audio, keyboard, mouse, joysti |
| 240 | `libebur128` | 1 | 2 | Library implementing the EBU R128 loudness standar |
| 241 | `octomap` | 1 | 0 | Efficient probabilistic 3D mapping framework based |
| 242 | `libtorrent-rakshasa` | 1 | 2 | BitTorrent library with a focus on high performanc |
| 243 | `xmlrpc-c` | 1 | 2 | Lightweight RPC library (based on XML and HTTP) |
| 244 | `krb5` | 1 | 2 | Network authentication protocol |
| 245 | `libxcrypt` | 1 | 0 | Extended crypt library for descrypt, md5crypt, bcr |
| 246 | `tdb` | 1 | 0 | Trivial DataBase, by the Samba project |
| 247 | `libxls` | 1 | 0 | Read binary Excel files from C/C++ |
| 248 | `a52dec` | 1 | 0 | Library for decoding ATSC A/52 streams (AKA 'AC-3' |
| 249 | `libmpeg2` | 1 | 2 | Library to decode mpeg-2 and mpeg-1 video streams |
| 250 | `gputils` | 1 | 0 | GNU PIC Utilities |
| 251 | `libconfig` | 1 | 0 | Configuration file processing library |
| 252 | `libdaemon` | 1 | 0 | C library that eases writing UNIX daemons |
| 253 | `ffms2` | 1 | 11 | Libav/ffmpeg based source library and Avisynth plu |
| 254 | `healpix` | 1 | 1 | Hierarchical Equal Area isoLatitude Pixelization o |
| 255 | `readosm` | 1 | 0 | Extract valid data from an Open Street Map input f |
| 256 | `spice-protocol` | 1 | 0 | Headers for SPICE protocol |
| 257 | `glm` | 1 | 0 | C++ mathematics library for graphics software |
| 258 | `libtpms` | 1 | 2 | Library for software emulation of a Trusted Platfo |
| 259 | `libmng` | 1 | 6 | MNG/JNG reference library |
| 260 | `ivykis` | 1 | 0 | Async I/O-assisting library |
| 261 | `libdbi` | 1 | 0 | Database-independent abstraction layer in C, simil |
| 262 | `magic_enum` | 1 | 0 | Static reflection for enums (to string, from strin |
| 263 | `fltk@1.3` | 1 | 2 | Cross-platform C++ GUI toolkit |
| 264 | `console_bridge` | 1 | 0 | Robot Operating System-independent package for log |
| 265 | `urdfdom_headers` | 1 | 0 | Headers for Unified Robot Description Format (URDF |
| 266 | `helm@3` | 1 | 0 | Kubernetes package manager |
| 267 | `libdc1394` | 1 | 2 | Provides API for IEEE 1394 cameras |
| 268 | `etcd` | 1 | 0 | Key value store for shared configuration and servi |
| 269 | `vulkan-headers` | 1 | 0 | Vulkan Header files and API registry |
| 270 | `lzlib` | 1 | 0 | Data compression library |
| 271 | `libsmi` | 1 | 0 | Library to Access SMI MIB Information |
| 272 | `z3` | 1 | 0 | High-performance theorem prover |
| 273 | `libvncserver` | 1 | 8 | VNC server and client libraries |
| 274 | `tcl-tk@8` | 1 | 2 | Tool Command Language |
| 275 | `xkeyboard-config` | 1 | 0 | Keyboard configuration database for the X Window S |

## Near-Leaves: Need Just 1 Missing Dep

These libraries are missing exactly one dependency. Creating their blocker
plus the library itself would unblock their dependents.

| Library | Dependents | Missing Dep |
|---|---|---|
| `python@3.14` | 36 | `mpdecimal` |
| `gdk-pixbuf` | 34 | `libtiff` |
| `harfbuzz` | 27 | `icu4c@78` |
| `boost` | 23 | `icu4c@78` |
| `openjdk` | 18 | `harfbuzz` |
| `libvorbis` | 17 | `libogg` |
| `fftw` | 13 | `libomp` |
| `webp` | 13 | `libtiff` |
| `adwaita-icon-theme` | 12 | `librsvg` |
| `libtool` | 10 | `m4` |
| `flac` | 9 | `libogg` |
| `gpgme` | 9 | `gnupg` |
| `openblas` | 7 | `libomp` |
| `openjpeg` | 7 | `libtiff` |
| `libsoup` | 6 | `glib-networking` |
| `taglib` | 6 | `utf8cpp` |
| `libmpc` | 5 | `mpfr` |
| `gobject-introspection` | 5 | `python@3.14` |
| `openjdk@21` | 4 | `harfbuzz` |
| `hdf5` | 4 | `libaec` |
| `zeromq` | 4 | `libsodium` |
| `freexl` | 4 | `minizip` |
| `sdl2_ttf` | 3 | `harfbuzz` |
| `tmux` | 3 | `utf8proc` |
| `htslib` | 3 | `libdeflate` |
| `dotnet` | 3 | `icu4c@78` |
| `glog` | 3 | `gflags` |
| `scalapack` | 3 | `openblas` |
| `qt5compat` | 3 | `icu4c@78` |
| `qtpositioning` | 3 | `qtserialport` |

## Cascade Analysis

If we add all leaf libraries, which new libraries become available?

### Wave 1: 275 libraries become available

| Library | Dependents |
|---|---|
| `libtiff` | 26 |
| `libusb` | 25 |
| `lz4` | 21 |
| `libomp` | 16 |
| `libogg` | 15 |
| `mpfr` | 13 |
| `icu4c@78` | 11 |
| `protobuf` | 11 |
| `libzip` | 11 |
| `libunistring` | 9 |
| `lame` | 8 |
| `mpg123` | 8 |
| `qtsvg` | 8 |
| `fmt` | 7 |
| `libsamplerate` | 7 |
| `jansson` | 7 |
| `libuv` | 7 |
| `pixman` | 7 |
| `opus` | 7 |
| `minizip` | 7 |
| `mad` | 6 |
| `libao` | 6 |
| `nettle` | 6 |
| `json-c` | 6 |
| `coreutils` | 5 |
| `eigen` | 5 |
| `openldap` | 5 |
| `libmaxminddb` | 5 |
| `portaudio` | 5 |
| `imath` | 5 |
| ... | 245 more |

### Wave 2: 68 libraries become available

| Library | Dependents |
|---|---|
| `python@3.14` | 36 |
| `gdk-pixbuf` | 34 |
| `harfbuzz` | 27 |
| `boost` | 23 |
| `libvorbis` | 17 |
| `libarchive` | 14 |
| `fftw` | 13 |
| `webp` | 13 |
| `libtool` | 10 |
| `flac` | 9 |
| `libpq` | 8 |
| `openblas` | 7 |
| `openjpeg` | 7 |
| `libmpc` | 5 |
| `openexr` | 5 |
| `opusfile` | 4 |
| `hdf5` | 4 |
| `zeromq` | 4 |
| `freexl` | 4 |
| `tmux` | 3 |
| `htslib` | 3 |
| `dotnet` | 3 |
| `glog` | 3 |
| `qt5compat` | 3 |
| `qtpositioning` | 3 |
| `libraw` | 3 |
| `docbook-xsl` | 3 |
| `autoconf` | 2 |
| `spdlog` | 2 |
| `wxwidgets@3.2` | 2 |
| ... | 38 more |

### Wave 3: 46 libraries become available

| Library | Dependents |
|---|---|
| `openjdk` | 18 |
| `sdl2_mixer` | 7 |
| `libxcb` | 6 |
| `gobject-introspection` | 5 |
| `imagemagick` | 5 |
| `openjdk@21` | 4 |
| `llvm` | 4 |
| `flann` | 4 |
| `libspatialite` | 4 |
| `sdl2_ttf` | 3 |
| `libftdi` | 3 |
| `scalapack` | 3 |
| `theora` | 3 |
| `qt@5` | 3 |
| `libint` | 2 |
| `erlang` | 2 |
| `libsm` | 2 |
| `qtwebengine` | 2 |
| `soapysdr` | 2 |
| `openjdk@11` | 1 |
| `yamale` | 1 |
| `ceres-solver` | 1 |
| `cgal` | 1 |
| `faiss` | 1 |
| `dbcsr` | 1 |
| `automake` | 1 |
| `libcanberra` | 1 |
| `slicot` | 1 |
| `collectd` | 1 |
| `qtlocation` | 1 |
| ... | 16 more |

### Wave 4: 13 libraries become available

| Library | Dependents |
|---|---|
| `libx11` | 17 |
| `petsc` | 1 |
| `yuicompressor` | 1 |
| `template-glib` | 1 |
| `pyqt@5` | 1 |
| `qwt-qt5` | 1 |
| `soapyrtlsdr` | 1 |
| `libshout` | 1 |
| `zbar` | 1 |
| `xcb-util` | 1 |
| `xcb-util-keysyms` | 1 |
| `xcb-util-renderutil` | 1 |
| `xcb-util-wm` | 1 |

### Wave 5: 5 libraries become available

| Library | Dependents |
|---|---|
| `libxext` | 7 |
| `libxt` | 6 |
| `libxrender` | 3 |
| `libxfixes` | 1 |
| `xcb-util-image` | 1 |

### Wave 6: 7 libraries become available

| Library | Dependents |
|---|---|
| `libxmu` | 3 |
| `libxft` | 2 |
| `gpac` | 1 |
| `libxrandr` | 1 |
| `libxinerama` | 1 |
| `libxi` | 1 |
| `libapplewm` | 1 |

### Wave 7: 3 libraries become available

| Library | Dependents |
|---|---|
| `libxaw` | 1 |
| `libxaw3d` | 1 |
| `xauth` | 1 |

### Summary: 7 waves, 417 libraries added

- Recipes with missing direct deps (before): 549
- Recipes fully unblocked (after cascading): 418
- Recipes still blocked: 131

**Still-blocked recipes (first 20):**

| Recipe | Remaining Missing Deps |
|---|---|
| `astrometry-net` | `netpbm` |
| `asymptote` | `ghostscript` |
| `audacious` | `qtimageformats` |
| `audiowaveform` | `gd` |
| `bazarr` | `pillow` |
| `cdogs-sdl` | `sdl2_image` |
| `cdxgen` | `node` |
| `colmap` | `openimageio` |
| `diff-pdf` | `poppler` |
| `dockerfilegraph` | `graphviz` |
| `dosbox-staging` | `sdl2_image` |
| `dynare` | `octave` |
| `emscripten` | `node` |
| `epstool` | `ghostscript` |
| `faircamp` | `vips` |
| `fancy-cat` | `mupdf` |
| `feedgnuplot` | `gnuplot` |
| `fontforge` | `pango` |
| `gammaray` | `graphviz` |
| `gopass` | `gnupg` |

## Recommended Priority Order

### Phase 1: High-impact leaf libraries (0 missing deps, >5 dependents)

**31 libraries, unblocking the most tools immediately:**

- `libtiff` (26 dependents) - TIFF library and utilities
- `libusb` (25 dependents) - Library for USB device access
- `lz4` (21 dependents) - Extremely Fast Compression algorithm
- `libomp` (16 dependents) - LLVM's OpenMP runtime library
- `libogg` (15 dependents) - Ogg Bitstream Library
- `mpfr` (13 dependents) - C library for multiple-precision floating-point computations
- `icu4c@78` (11 dependents) - C/C++ and Java libraries for Unicode and globalization
- `protobuf` (11 dependents) - Protocol buffers (Google's data interchange format)
- `libzip` (11 dependents) - C library for reading, creating, and modifying zip archives
- `libunistring` (9 dependents) - C string library for manipulating Unicode strings
- `lame` (8 dependents) - High quality MPEG Audio Layer III (MP3) encoder
- `mpg123` (8 dependents) - MP3 player for Linux and UNIX
- `qtsvg` (8 dependents) - Classes for displaying the contents of SVG files
- `fmt` (7 dependents) - Open-source formatting library for C++
- `libsamplerate` (7 dependents) - Library for sample rate conversion of audio data
- `jansson` (7 dependents) - C library for encoding, decoding, and manipulating JSON
- `libuv` (7 dependents) - Multi-platform support library with a focus on asynchronous 
- `pixman` (7 dependents) - Low-level library for pixel manipulation
- `opus` (7 dependents) - Audio codec
- `minizip` (7 dependents) - C library for zip/unzip via zLib
- `mad` (6 dependents) - MPEG audio decoder
- `libao` (6 dependents) - Cross-platform Audio Library
- `nettle` (6 dependents) - Low-level cryptographic library
- `json-c` (6 dependents) - JSON parser for C
- `coreutils` (5 dependents) - GNU File, Shell, and Text utilities
- `eigen` (5 dependents) - C++ template library for linear algebra
- `openldap` (5 dependents) - Open source suite of directory software
- `libmaxminddb` (5 dependents) - C library for the MaxMind DB file format
- `portaudio` (5 dependents) - Cross-platform library for audio I/O
- `imath` (5 dependents) - Library of 2D and 3D vector, matrix, and math operations
- `libpcap` (5 dependents) - Portable library for network traffic capture

### Phase 2: Medium-impact leaf libraries (0 missing deps, 2-4 dependents)

**79 libraries:**

- `xxhash` (4 dependents) - Extremely fast non-cryptographic hash algorithm
- `zlib-ng-compat` (4 dependents) - Zlib replacement with optimizations for next generation syst
- `libsoxr` (4 dependents) - High quality, one-dimensional sample-rate conversion library
- `qtmultimedia` (4 dependents) - Provides APIs for playing back and recording audiovisual con
- `lzo` (4 dependents) - Real-time data compression library
- `gflags` (4 dependents) - Library for processing command-line flags
- `llvm@21` (4 dependents) - Next-gen compiler infrastructure
- `hiredis` (4 dependents) - Minimalistic client for Redis
- `yaml-cpp` (4 dependents) - C++ YAML parser and emitter for YAML 1.2 spec
- `libexif` (4 dependents) - EXIF parsing library
- `popt` (4 dependents) - Library like getopt(3) with a number of enhancements
- `qhull` (4 dependents) - Computes convex hulls in n dimensions
- `librttopo` (4 dependents) - RT Topology Library
- `libtommath` (4 dependents) - C library for number theoretic multiple-precision integers
- `isl` (3 dependents) - Integer Set Library for the polyhedral model
- `berkeley-db@5` (3 dependents) - High performance key/value database
- `wcslib` (3 dependents) - Library and utilities for the FITS World Coordinate System
- `libmodplug` (3 dependents) - Library from the Modplug-XMMS project
- `wavpack` (3 dependents) - Hybrid lossless audio compression
- `hwloc` (3 dependents) - Portable abstraction of the hierarchical topology of modern 
- `libmagic` (3 dependents) - Implementation of the file(1) command
- `libsodium` (3 dependents) - NaCl networking and cryptography library
- `graphene` (3 dependents) - Thin layer of graphic data types
- `speexdsp` (3 dependents) - Speex audio processing library
- `x264` (3 dependents) - H.264/AVC encoder
- `libvpx` (3 dependents) - VP8/VP9 video codec
- `talloc` (3 dependents) - Hierarchical, reference-counted memory pool with destructors
- `glfw` (3 dependents) - Multi-platform library for OpenGL applications
- `hicolor-icon-theme` (3 dependents) - Fallback theme for FreeDesktop.org icon themes
- `dbus` (3 dependents) - Message bus system, providing inter-application communicatio
- `miniupnpc` (3 dependents) - UPnP IGD client library and daemon
- `lmdb` (3 dependents) - Lightning memory-mapped database: key-value data store
- `libimobiledevice-glue` (3 dependents) - Library with common system API code for libimobiledevice pro
- `tinyxml2` (3 dependents) - Improved tinyxml (in memory efficiency and size)
- `net-snmp` (3 dependents) - Implements SNMP v1, v2c, and v3, using IPv4 and IPv6
- `qtserialport` (3 dependents) - Provides classes to interact with hardware and virtual seria
- `libbs2b` (2 dependents) - Bauer stereophonic-to-binaural DSP
- `leveldb` (2 dependents) - Key-value storage library with ordered mapping
- `utf8proc` (2 dependents) - Clean C library for processing UTF-8 Unicode data
- `llvm@20` (2 dependents) - Next-gen compiler infrastructure
- `sdl2_net` (2 dependents) - Small sample cross-platform networking library
- `metis` (2 dependents) - Programs that partition graphs and order matrices
- `argon2` (2 dependents) - Password hashing library and CLI utility
- `concurrencykit` (2 dependents) - Aid design and implementation of concurrent systems
- `libslirp` (2 dependents) - General purpose TCP-IP emulator
- `woff2` (2 dependents) - Utilities to create and convert Web Open Font File (WOFF) fi
- `libxmp` (2 dependents) - C library for playback of module music (MOD, S3M, IT, etc)
- `inih` (2 dependents) - Simple .INI file parser in C
- `qtcharts` (2 dependents) - UI Components for displaying visually pleasing charts
- `libgee` (2 dependents) - Collection library providing GObject-based interfaces
- `libnet` (2 dependents) - C library for creating IP packets
- `yyjson` (2 dependents) - High performance JSON library written in ANSI C
- `aom` (2 dependents) - Codec library for encoding and decoding AV1 video streams
- `frei0r` (2 dependents) - Minimalistic plugin API for video effects
- `libssh` (2 dependents) - C library SSHv1/SSHv2 client and server protocols
- `libvidstab` (2 dependents) - Transcode video stabilization plugin
- `srt` (2 dependents) - Secure Reliable Transport
- `libcbor` (2 dependents) - CBOR protocol implementation for C and others
- `usb.ids` (2 dependents) - Repository of vendor, device, subsystem and device class IDs
- `qtconnectivity` (2 dependents) - Provides access to Bluetooth hardware
- `qtscxml` (2 dependents) - Provides functionality to create state machines from SCXML f
- `qtwebchannel` (2 dependents) - Bridges the gap between Qt applications and HTML/JavaScript
- `mbedtls@3` (2 dependents) - Cryptographic & SSL/TLS library
- `libiconv` (2 dependents) - Conversion library
- `terminal-notifier` (2 dependents) - Send macOS User Notifications from the command-line
- `libsigc++@2` (2 dependents) - Callback framework for C++
- `docbook` (2 dependents) - Standard XML representation system for technical documents
- `liblqr` (2 dependents) - C/C++ seam carving library
- `cjson` (2 dependents) - Ultralightweight JSON parser in ANSI C
- `libmicrohttpd` (2 dependents) - Light HTTP/1.1 server library
- `libepoxy` (2 dependents) - Library for handling OpenGL function pointer management
- `game-music-emu` (2 dependents) - Videogame music file emulator collection
- `libev` (2 dependents) - Asynchronous event library
- `tokyo-cabinet` (2 dependents) - Lightweight database library
- `capstone` (2 dependents) - Multi-platform, multi-architecture disassembly framework
- `pandoc` (2 dependents) - Swiss-army knife of markup format conversion
- `nss` (2 dependents) - Libraries for security-enabled client and server application
- `molten-vk` (2 dependents) - Implementation of the Vulkan graphics and compute API on top
- `libxlsxwriter` (2 dependents) - C library for creating Excel XLSX files

### Phase 3: Remaining leaf libraries (0 missing deps, 1 dependent)

**165 libraries** (create as needed when their dependent is requested)
