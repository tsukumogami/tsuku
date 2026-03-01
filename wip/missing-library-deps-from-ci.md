# Missing Library Dependencies from CI Analysis

Analysis of macOS CI test-recipe failures on branch `docs/system-lib-backfill`.

**Source data:**
- Run 22534058918 (2026-03-01, 67 batches, most comprehensive -- in progress during analysis)
- Run 22273383159 (2026-02-22, earlier smaller run with both arm64 and x86_64 failures)

**Summary:**
- 328 unique tool/platform failures across macOS arm64 and x86_64
- 176 unique tool/platform passes
- 3 distinct failure categories identified

---

## Category 1: dyld errors -- tools need `runtime_dependencies` added

These tools install successfully but crash during verification because they link against a
library that wasn't declared in their recipe. The fix is adding `runtime_dependencies` to
each tool's recipe TOML.

### Tools needing existing library recipes as runtime_dependencies

These library recipes already exist (or exist as internal recipes). The tool recipes just
need `runtime_dependencies = [...]` declarations added.

| Failed Tool | Missing dylib | Library Recipe | Platforms |
|---|---|---|---|
| aoe | libssl.3.dylib | openssl | macOS arm64, macOS x86_64 |
| aws-lc | libssl.dylib | openssl | macOS arm64 |
| code-cli | libssl.3.dylib | openssl | macOS arm64, macOS x86_64 |
| dnsperf | libssl.3.dylib | openssl | macOS arm64 |
| git-crypt | libcrypto.3.dylib | openssl | macOS arm64 |
| git-xet | libssl.3.dylib | openssl | macOS arm64 |
| gitlogue | libssl.3.dylib | openssl | macOS arm64 |
| openfortivpn | libssl.3.dylib | openssl | macOS arm64 |
| aarch64-elf-gcc | libzstd.1.dylib | zstd | macOS arm64, macOS x86_64 |
| arm-linux-gnueabihf-binutils | libzstd.1.dylib | zstd | macOS arm64, macOS x86_64 |
| arm-none-eabi-binutils | libzstd.1.dylib | zstd | macOS arm64, macOS x86_64 |
| avro-cpp | libzstd.1.dylib | zstd | macOS arm64 |
| riscv64-elf-binutils | libzstd.1.dylib | zstd | macOS arm64 |
| aarch64-elf-gdb | liblzma.5.dylib | xz | macOS arm64 |
| arm-none-eabi-gdb | liblzma.5.dylib | xz | macOS arm64 |
| bedtools | liblzma.5.dylib | xz | macOS arm64 |
| dar | liblzma.5.dylib | xz | macOS arm64 |
| payload-dumper-go | liblzma.5.dylib | xz | macOS arm64 |
| riscv64-elf-gdb | liblzma.5.dylib | xz | macOS arm64 |
| rpm2cpio | liblzma.5.dylib | xz | macOS arm64 |
| calcurse | libintl.8.dylib | gettext | macOS arm64 |
| homebank | libintl.8.dylib | gettext | macOS arm64 |
| libxpm | libintl.8.dylib | gettext | macOS arm64 |
| newsboat | libintl.8.dylib | gettext | macOS arm64 |
| aircrack-ng | libreadline.8.dylib | readline | macOS arm64 |
| bigloo | libreadline.8.dylib | readline | macOS arm64 |
| cdxgen | libreadline.8.dylib | readline | macOS arm64 |
| oils-for-unix | libreadline.8.dylib | readline | macOS arm64 |
| bnfc | libgmp.10.dylib | gmp | macOS arm64 |
| cabal-install | libgmp.10.dylib | gmp | macOS arm64 |
| futhark | libgmp.10.dylib | gmp | macOS arm64 |
| hledger | libgmp.10.dylib | gmp | macOS arm64 |
| colmap | libGLEW.2.3.dylib | glew | macOS arm64, macOS x86_64 |
| gource | libGLEW.2.3.dylib | glew | macOS arm64, macOS x86_64 |
| pcl | libGLEW.2.3.dylib | glew | macOS arm64 |
| a2ps | libgc.1.dylib | bdw-gc | macOS arm64, macOS x86_64 |
| jp2a | libjpeg.8.dylib | jpeg-turbo | macOS arm64 |
| jpeginfo | libjpeg.8.dylib | jpeg-turbo | macOS arm64 |
| google-authenticator-libpam | libqrencode.4.dylib | qrencode | macOS arm64, macOS x86_64 |
| audiowaveform | libsndfile.1.dylib | libsndfile | macOS arm64 |
| bulk-extractor | libabsl_bad_optional_access.2401.0.0.dylib | abseil | macOS arm64 |
| drogon | libcares.2.dylib | c-ares | macOS arm64 |
| rsgain | libavformat.61.dylib | ffmpeg | macOS arm64 |
| cdogs-sdl | libSDL2-2.0.0.dylib | sdl2 | macOS arm64 |
| chapel | libpkgconf.7.dylib | pkgconf | macOS arm64 |

### Tools needing library recipes that DO NOT EXIST yet

These require new library recipes to be created first, then declared as runtime_dependencies
in the tool recipe.

| Failed Tool | Missing dylib | Library Recipe (needs creation) | Platforms |
|---|---|---|---|
| dfu-programmer | libusb-1.0.0.dylib | libusb | macOS arm64 |
| dfu-util | libusb-1.0.0.dylib | libusb | macOS arm64 |
| openfpgaloader | libusb-1.0.0.dylib | libusb | macOS arm64 |
| goaccess | libmaxminddb.0.dylib | libmaxminddb | macOS arm64, macOS x86_64 |
| doltgres | libicui18n.78.dylib | icu4c | macOS arm64 |
| gastown | libicui18n.78.dylib | icu4c | macOS arm64 |
| argyll-cms | libtiff.6.dylib | libtiff | macOS arm64, macOS x86_64 |
| astroterm | libargtable3.0.dylib | argtable | macOS arm64 |
| ncmpcpp | libboost_date_time-mt.dylib | boost | macOS arm64 |
| ngspice | libfftw3.3.dylib | fftw | macOS arm64 |
| brpc | libgflags.2.3.dylib | gflags | macOS arm64 |
| fwup | libconfuse.2.dylib | libconfuse | macOS arm64 |
| cdrdao | libmad.0.dylib | libmad | macOS arm64 |
| par2 | libomp.dylib | libomp | macOS arm64 |
| ngrep | libpcap.A.dylib | libpcap | macOS arm64 |
| rlwrap | libptytty.0.dylib | libptytty | macOS arm64 |
| bochs | libltdl.7.dylib | libtool | macOS arm64 |
| katago | libzip.5.dylib | libzip | macOS arm64 |
| ldc | libLLVM.dylib | llvm | macOS arm64 |
| neovim-qt | libmsgpack-c.2.dylib | msgpack-cxx | macOS arm64 |
| open-simh | libpcre.1.dylib | pcre | macOS arm64 |
| damask-grid | libpetsc.3.23.dylib | petsc | macOS arm64 |
| rom-tools | libSDL3.0.dylib | sdl3 | macOS arm64 |

---

## Category 2: registry errors -- dependency recipe not found

These tools declare a `runtime_dependencies` entry that points to a recipe not present in
the registry at the tested commit. Most of these recipes exist on the current HEAD of the
branch (they were added in later commits or in the same PR but after the batch was queued).

| Failed Tool | Missing Recipe | Platforms |
|---|---|---|
| ddcutil | glib | macOS arm64 |
| desktop-file-utils | glib | macOS arm64 |
| diff-pdf | glib | macOS arm64 |
| dissent | glib | macOS arm64 |
| fwupd | glib | macOS arm64 |
| gabedit | glib | macOS arm64 |
| gedit | glib | macOS arm64 |
| gensio | glib | macOS arm64 |
| gerbv | glib | macOS arm64 |
| git-credential-libsecret | glib | macOS arm64 |
| gkrellm | glib | macOS arm64 |
| json-glib | glib | macOS arm64 |
| libxmlb | glib | macOS arm64 |
| pdf2svg | glib | macOS arm64 |
| pdfpc | glib | macOS arm64 |
| rmlint | glib | macOS arm64 |
| ldid | libplist | macOS arm64 |
| ldid-procursus | libplist | macOS arm64 |
| libusbmuxd | libplist | macOS arm64 |
| hdf5-mpi | open-mpi | macOS arm64 |
| opencoarrays | open-mpi | macOS arm64 |
| netatalk | libevent | macOS arm64 |
| open-mpi | libevent | macOS arm64 |
| asymptote | gsl | macOS arm64 |
| bcftools | gsl | macOS arm64 |
| avrdude | hidapi | macOS arm64 |
| open-ocd | hidapi | macOS arm64 |
| dosbox-staging | fluid-synth | macOS arm64 |
| dosbox-x | fluid-synth | macOS arm64 |
| apt | bzip2 | macOS arm64, macOS x86_64 |
| arm-none-eabi-gcc | arm-none-eabi-binutils | macOS arm64, macOS x86_64 |
| aerc | notmuch | macOS arm64, macOS x86_64 |
| gopass-jsonapi | gopass | macOS arm64, macOS x86_64 |
| riscv64-elf-gcc | riscv64-elf-binutils | macOS arm64 |
| astrometry-net | cfitsio | macOS arm64 |
| audacious | faad2 | macOS arm64 |
| avro-c | snappy | macOS arm64 |
| bazarr | numpy | macOS arm64 |
| darkice | faac | macOS arm64 |
| geeqie | djvulibre | macOS arm64 |
| get-iplayer | atomicparsley | macOS arm64 |
| jpeg-xl | little-cms2 | macOS arm64 |
| jq | oniguruma | macOS arm64 |
| lanraragi | redis | macOS arm64 |
| libxmlsec1 | gnutls | macOS arm64 |
| neomutt | libidn2 | macOS arm64 |
| pcb2gcode | gerbv | macOS arm64 |
| pdfgrep | libgcrypt | macOS arm64 |
| rizin | tree-sitter | macOS arm64 |

**Note:** All 27 unique missing recipes in this category DO exist on the current branch HEAD.
These failures are from an earlier commit or from the batch being queued before the recipes
were added.

---

## Category 3: self-referencing dyld errors -- packaging/relocation issues

These tools load their own shared library via `@rpath` and fail. This is not about missing
dependencies but about the Homebrew bottle relocation not correctly setting up the library
search path for the tool's own libraries.

| Tool | Own Library Not Found |
|---|---|
| afflib | libafflib.0.dylib |
| aspell | libaspell.15.dylib |
| baresip | libbaresip.25.dylib |
| bareos-client | libbareos.25.dylib |
| build2 | libbuild2-0.17.dylib |
| cfitsio | libcfitsio.10.dylib |
| cgns | libcgns.4.5.dylib |
| cmark | libcmark.0.31.2.dylib |
| dcmtk | libdcmdata.20.dylib |
| dovecot | libdovecot-storage.0.dylib |
| freetds | libsybdb.5.dylib |
| fribidi | libfribidi.0.dylib |
| gauche | libgauche-0.98.dylib |
| ginac | libginac.13.dylib |
| ldns | libldns.3.dylib |
| libsixel | libsixel.1.dylib |
| libsndfile | libsndfile.1.dylib |
| libtasn1 | libtasn1.6.dylib |
| libvterm | libvterm.0.dylib |
| libxc | libxc.15.dylib |
| lighttpd | liblightcomp.dylib |
| limesuite | libLimeSuite.23.11-1.dylib |
| omniorb | libomniORB4.3.dylib |
| open-image-denoise | libOpenImageDenoise.2.dylib |
| openal-soft | libopenal.1.dylib |

These need a different fix (improving `homebrew_relocate` to handle self-referencing dylibs)
rather than adding library dependencies.

---

## Library recipes that need to be CREATED

Deduplicated list of all unique library recipes that don't exist yet and are needed as
`runtime_dependencies` by tool recipes that currently fail with dyld errors.

| Library Recipe | Homebrew Formula | Tools Unblocked | Priority |
|---|---|---|---|
| libusb | libusb | 3 | High |
| icu4c | icu4c@76 | 2 | High |
| libtiff | libtiff | 1 | Medium |
| argtable | argtable | 1 | Low |
| boost | boost | 1 | Low |
| fftw | fftw | 1 | Low |
| gflags | gflags | 1 | Low |
| libconfuse | libconfuse | 1 | Low |
| libmad | mad | 1 | Low |
| libmaxminddb | libmaxminddb | 1 | Low |
| libomp | libomp | 1 | Low |
| libpcap | libpcap | 1 | Low |
| libptytty | libptytty | 1 | Low |
| libtool | libtool | 1 | Low |
| libzip | libzip | 1 | Low |
| llvm | llvm | 1 | Low |
| msgpack-cxx | msgpack-cxx | 1 | Low |
| pcre | pcre | 1 | Low |
| petsc | petsc | 1 | Low |
| sdl3 | sdl3 | 1 | Low |

**Total: 20 library recipes need creation.**

---

## Existing library recipes that need `runtime_dependencies` declarations

These existing library recipes need to be added as `runtime_dependencies` to the tool
recipes listed above (Category 1, first table). Sorted by number of tools affected.

| Library Recipe | Tools Needing It as runtime_dependency |
|---|---|
| openssl | 8 (aoe, aws-lc, code-cli, dnsperf, git-crypt, git-xet, gitlogue, openfortivpn) |
| xz | 7 (aarch64-elf-gdb, arm-none-eabi-gdb, bedtools, dar, payload-dumper-go, riscv64-elf-gdb, rpm2cpio) |
| zstd | 5 (aarch64-elf-gcc, arm-linux-gnueabihf-binutils, arm-none-eabi-binutils, avro-cpp, riscv64-elf-binutils) |
| gettext | 4 (calcurse, homebank, libxpm, newsboat) |
| gmp | 4 (bnfc, cabal-install, futhark, hledger) |
| readline | 4 (aircrack-ng, bigloo, cdxgen, oils-for-unix) |
| glew | 3 (colmap, gource, pcl) |
| jpeg-turbo | 2 (jp2a, jpeginfo) |
| bdw-gc | 1 (a2ps) |
| abseil | 1 (bulk-extractor) |
| c-ares | 1 (drogon) |
| ffmpeg | 1 (rsgain) |
| libsndfile | 1 (audiowaveform) |
| pkgconf | 1 (chapel) |
| qrencode | 1 (google-authenticator-libpam) |
| sdl2 | 1 (cdogs-sdl) |
