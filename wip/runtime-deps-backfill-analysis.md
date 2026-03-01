# Runtime Dependencies Backfill Analysis

Analysis of Homebrew recipes missing `runtime_dependencies` entries,
caused by `scripts/fix-recipe-errors.py` stripping deps that didn't match
existing recipe names in the registry (see commit `efbd9ac2`).

**Methodology**: Compared the Homebrew API's `dependencies` array for each
formula against the `runtime_dependencies` declared in our recipe TOML files.
API data sourced from `https://formulae.brew.sh/api/formula.json` (bulk endpoint).

## Summary Statistics

| Metric | Count |
|--------|-------|
| Total Homebrew recipes in registry | 1,282 |
| Recipes with no API deps (no action needed) | 653 |
| Recipes already correct (deps match API) | 108 |
| **Recipes with missing deps** | **521** |
| - Fully backfillable now (all dep recipes exist) | 13 |
| - Partially blocked (some dep recipes missing) | 508 |
| Unique dep names where recipe exists | 66 |
| Unique dep names where no recipe exists | 507 |
| Total dep-recipe pairs to backfill (recipe exists) | 126 |
| Total dep-recipe pairs blocked (no recipe) | 1581 |

### Note on `uses_from_macos`

The Homebrew API also has a `uses_from_macos` field listing deps provided by
macOS (like `ncurses`, `curl`, `libxml2`). These are only needed on Linux.
351 of our Homebrew recipes have `uses_from_macos` entries (39 unique deps).
This analysis focuses only on the `dependencies` field, which represents
unconditional runtime deps. The `uses_from_macos` backfill is a separate task.

## Library Recipes That Need to Be Created

507 unique dependency names reference formulas with no recipe in the registry.
Sorted by number of tool recipes that depend on them (highest impact first).

### High Impact (10+ dependents)

| Dep Name | Dependents | Sample Recipes |
|----------|-----------|----------------|
| openssl@3 | 137 | afflib, aircrack-ng, aoe, apr-util, apt, argyll-cms, ... (+131 more) |
| python@3.14 | 36 | aarch64-elf-gdb, arm-none-eabi-gdb, astrometry-net, bazarr, bento4, cdogs-sdl, ... (+30 more) |
| pango | 35 | claws-mail, dissent, eiffelstudio, enter-tex, ettercap, fontforge, ... (+29 more) |
| gdk-pixbuf | 34 | audacious, claws-mail, dissent, eiffelstudio, enter-tex, ettercap, ... (+28 more) |
| harfbuzz | 27 | claws-mail, dissent, easyrpg-player, eiffelstudio, ffmpeg-full, freeciv, ... (+21 more) |
| libtiff | 26 | argyll-cms, dcmtk, djvulibre, fontforge, geeqie, gnome-papers, ... (+20 more) |
| libusb | 25 | avrdude, ddcutil, dfu-programmer, dfu-util, fwupd, libbladerf, ... (+19 more) |
| gtk+3 | 24 | claws-mail, eiffelstudio, enter-tex, ettercap, freeciv, gedit, ... (+18 more) |
| boost | 23 | audiowaveform, avro-cpp, colmap, cryfs, fastnetmon, gnuradio, ... (+17 more) |
| at-spi2-core | 22 | claws-mail, eiffelstudio, ettercap, freeciv, gabedit, gerbv, ... (+16 more) |
| lz4 | 21 | apt, colmap, dar, librasterlite2, micromamba, osmcoastline, ... (+15 more) |
| openjdk | 18 | alda, apache-polaris, bbtools, bigloo, cljfmt, emscripten, ... (+12 more) |
| libvorbis | 17 | audacious, cdrdao, darkice, easyrpg-player, ffmpeg-full, frotz, ... (+11 more) |
| libx11 | 17 | cairo, ddcutil, eiffelstudio, feh, ffmpeg-full, fricas, ... (+11 more) |
| libomp | 16 | colmap, cp2k, damask-grid, dynare, gromacs, imagemagick-full, ... (+10 more) |
| libogg | 15 | audacious, darkice, easyrpg-player, ffmpeg-full, libopenmpt, libsndfile, ... (+9 more) |
| gcc | 14 | apt, cgns, cp2k, damask-grid, dynare, gambit-scheme, ... (+8 more) |
| libarchive | 14 | fceux, ffmpeg-full, fwup, fwupd, geeqie, gnome-papers, ... (+8 more) |
| fftw | 13 | asymptote, cp2k, damask-grid, gnuradio, gromacs, gwyddion, ... (+7 more) |
| webp | 13 | ffmpeg-full, geeqie, imagemagick-full, jbig2enc, jp2a, leptonica, ... (+7 more) |
| mpfr | 12 | aarch64-elf-gcc, aarch64-elf-gdb, arm-none-eabi-gcc, arm-none-eabi-gdb, colmap, i386-elf-gdb, ... (+6 more) |
| adwaita-icon-theme | 12 | enter-tex, freeciv, gedit, geeqie, gnome-builder, gnome-papers, ... (+6 more) |
| icu4c@78 | 11 | doltgres, easyrpg-player, freeciv, gastown, ncmpcpp, percona-server, ... (+5 more) |
| protobuf | 11 | fastnetmon, percona-server, percona-xtrabackup, protobuf-c, protoc-gen-doc, protoc-gen-go, ... (+5 more) |
| libzip | 11 | ideviceinstaller, imagemagick-full, katago, mgba, php, ppsspp, ... (+5 more) |
| ghostscript | 10 | asymptote, dvisvgm, epstool, fig2dev, groff, imagemagick-full, ... (+4 more) |
| libtool | 10 | bochs, crosstool-ng, fontforge, gnuastro, guile, imagemagick-full, ... (+4 more) |

### Medium Impact (4-9 dependents)

| Dep Name | Dependents | Recipes |
|----------|-----------|---------|
| flac | 9 | audacious, libopenmpt, libsndfile, qmmp, rom-tools, scummvm, scummvm-tools, sox-ng, ... (+1 more) |
| libunistring | 9 | bigloo, gettext, gnutls, guile, libidn2, libpsl, lnav, mailutils, ... (+1 more) |
| gpgme | 9 | kubekey, mutt, neomutt, operator-sdk, poppler-qt5, reprepro, skopeo, umoci, ... (+1 more) |
| mpg123 | 8 | audacious, easyrpg-player, libopenmpt, libsndfile, musikcube, qmmp, undercutf1, vgmstream |
| lame | 8 | audacious, cdrdao, darkice, ffmpeg, ffmpeg-full, libsndfile, musikcube, sox-ng |
| qtsvg | 8 | audacious, ecflow-ui, gammaray, mlt, neovim-qt, pyqt, qtdeclarative, rtabmap |
| libpq | 8 | coturn, mapserver, osm2pgrouting, osm2pgsql, pgbackrest, php, sleuthkit, spatialite-gui |
| poppler | 8 | diff-pdf, geeqie, gnome-papers, pdf2svg, pdfgrep, pdfpc, pdftoipe, pqiv |
| fmt | 7 | ada-url, ccache, cryfs, easyrpg-player, gnuradio, micromamba, rsgain |
| libsamplerate | 7 | audacious, chocolate-doom, darkice, ffmpeg-full, frotz, mlt, qmmp |
| jansson | 7 | avro-c, bareos-client, bedops, ddcutil, libjwt, powerman, universal-ctags |
| libuv | 7 | bigloo, knot-resolver, llgo, moarvm, rakudo-star, ttyd, zeek |
| pixman | 7 | cairo, easyrpg-player, libgr, librasterlite2, spice-gtk, tiger-vnc, xorg-server |
| libxext | 7 | cairo, ddcutil, imlib2, ngspice, rxvt-unicode, xeyes, xorg-server |
| sdl2_mixer | 7 | cdogs-sdl, chocolate-doom, corsixth, fheroes2, freeciv, frotz, widelands |
| openblas | 7 | cp2k, damask-grid, dynare, gromacs, jags, nwchem, visp |
| opus | 7 | faircamp, ffmpeg, ffmpeg-full, libsndfile, qmmp, sox-ng, spice-gtk |
| minizip | 7 | fceux, gwyddion, librasterlite2, spatialite, spatialite-gui, spatialite-tools, widelands |
| openjpeg | 7 | ffmpeg-full, geeqie, imagemagick-full, leptonica, librasterlite2, poppler-qt5, spatialite-gui |
| mad | 6 | audiowaveform, cdrdao, qmmp, scummvm, scummvm-tools, sox-ng |
| libxcb | 6 | cairo, ffmpeg-full, gnu-apl, imlib2, xeyes, xorg-server |
| sdl2_image | 6 | cdogs-sdl, dosbox-staging, gource, logstalgia, supertux, widelands |
| libao | 6 | cdrdao, frotz, pianobar, shairport-sync, vgmstream, vorbis-tools |
| node | 6 | cdxgen, emscripten, jhipster, lanraragi, phoneinfoga, tailwindcss-language-server |
| nettle | 6 | chrony, claws-mail, gnutls, hopenpgp-tools, rdfind, tiger-vnc |
| gtk4 | 6 | dissent, gnome-builder, gnome-papers, gplugin, gtranslator, nip4 |
| libxt | 6 | feh, fricas, ngspice, rxvt-unicode, xeyes, xfig |
| json-c | 6 | freeradius-server, gnucobol, newsboat, pianobar, syslog-ng, ttyd |
| gtk+ | 6 | gabedit, gerbv, gkrellm, gtk-gnutella, gwyddion, pcb2gcode |
| libsoup | 6 | gtranslator, gupnp, gupnp-tools, homebank, libosinfo, spice-gtk |
| taglib | 6 | musikcube, navidrome, ncmpcpp, pianod, qmmp, rsgain |
| netpbm | 5 | astrometry-net, fig2dev, groff, latex2html, siril |
| gd | 5 | audiowaveform, libgphoto2, php, pstoedit, vnstat |
| coreutils | 5 | clazy, crosstool-ng, elan-init, joern, pyenv-virtualenv |
| eigen | 5 | colmap, lc0, pcl, visp, votca |
| gobject-introspection | 5 | dissent, gedit, gnome-builder, mikutter, spice-gtk |
| openldap | 5 | dovecot, lighttpd, netatalk, percona-server, php |
| libmaxminddb | 5 | ettercap, goaccess, syslog-ng, wireshark, zeek |
| portaudio | 5 | fluid-synth, gensio, gnuradio, libopenmpt, musikcube |
| imath | 5 | geeqie, imagemagick-full, jpeg-xl, povray, synfig |
| openexr | 5 | geeqie, imagemagick-full, jpeg-xl, povray, synfig |
| imagemagick | 5 | geeqie, lanraragi, pqiv, pstoedit, synfig |
| librsvg | 5 | imagemagick-full, pdfpc, pioneers, qdmr, siril |
| libpcap | 5 | ngrep, pcl, rtabmap, tcpdump, visp |
| libmpc | 4 | aarch64-elf-gcc, arm-none-eabi-gcc, i686-elf-gcc, riscv64-elf-gcc |
| zlib-ng-compat | 4 | apt, mysql-client, percona-server, percona-xtrabackup |
| xxhash | 4 | apt, bareos-client, ccache, rizin |
| qtmultimedia | 4 | audacious, pc6001vx, pyqt, qmmp |
| opusfile | 4 | audacious, dosbox-staging, qmmp, sox-ng |
| libsoxr | 4 | audacious, ffmpeg-full, qmmp, shairport-sync |
| lzo | 4 | bareos-client, cairo, dar, squashfs |
| openjdk@21 | 4 | bazel, ghidra, kotlin-language-server, ltex-ls |
| gflags | 4 | brpc, colmap, libgrape-lite, librime |
| llvm@21 | 4 | c3c, clang-uml, include-what-you-use, spirv-llvm-translator |
| hiredis | 4 | ccache, coturn, fastnetmon, syslog-ng |
| llvm | 4 | ccls, clazy, gnome-builder, lld |
| hdf5 | 4 | cgns, libmatio, sratoolkit, votca |
| yaml-cpp | 4 | clang-uml, librime, micromamba, qdmr |
| flann | 4 | colmap, pcl, rtabmap, visp |
| libspelling | 4 | dissent, gnome-builder, gnome-papers, gtranslator |
| libadwaita | 4 | dissent, gnome-builder, gnome-papers, gtranslator |
| gtksourceview5 | 4 | dissent, gnome-builder, gnome-papers, gtranslator |
| graphviz | 4 | dockerfilegraph, gammaray, msc-generator, qcachegrind |
| gnuplot | 4 | feedgnuplot, libqalculate, limesuite, siril |
| libexif | 4 | feh, jp2a, libgphoto2, mlt |
| zeromq | 4 | ffmpeg-full, gnuradio, libgr, zeek |
| popt | 4 | gptfdisk, rabbitmq-c, samba, shairport-sync |
| qhull | 4 | libgr, pcl, rtabmap, visp |
| freexl | 4 | librasterlite2, spatialite, spatialite-gui, spatialite-tools |
| libspatialite | 4 | librasterlite2, osmcoastline, spatialite-gui, spatialite-tools |
| librttopo | 4 | librasterlite2, spatialite, spatialite-gui, spatialite-tools |
| opencv | 4 | mlt, rtabmap, siril, visp |
| libtommath | 4 | moarvm, rakudo-star, sqlite-analyzer, tcl-tk |

### Low Impact (2-3 dependents)

| Dep Name | Dependents | Recipes |
|----------|-----------|---------|
| sdl2_ttf | 3 | allureofthestars, openmsx, widelands |
| tmux | 3 | aoe, gitmux, tmuxai |
| berkeley-db@5 | 3 | apt, netatalk, reprepro |
| systemd | 3 | apt, ddcutil, onedrive-cli |
| wcslib | 3 | astrometry-net, gnuastro, siril |
| wavpack | 3 | audacious, qmmp, sox-ng |
| libmodplug | 3 | audacious, frotz, qmmp |
| libftdi | 3 | avrdude, open-ocd, openfpgaloader |
| htslib | 3 | bcftools, samtools, vcftools |
| libxrender | 3 | cairo, rxvt-unicode, xeyes |
| dotnet | 3 | cdxgen, kiota, undercutf1 |
| hwloc | 3 | chapel, nwchem, open-mpi |
| libmagic | 3 | clifm, file-formula, rizin |
| glog | 3 | colmap, libgrape-lite, librime |
| libsodium | 3 | core-lightning, php, pure-ftpd |
| scalapack | 3 | cp2k, damask-grid, nwchem |
| graphene | 3 | dissent, gnome-papers, nip4 |
| speexdsp | 3 | dosbox-staging, easyrpg-player, wireshark |
| qt5compat | 3 | ecflow-ui, mlt, qca |
| gspell | 3 | enter-tex, gedit, geeqie |
| x264 | 3 | fceux, ffmpeg, ffmpeg-full |
| libvpx | 3 | ffmpeg, ffmpeg-full, scummvm |
| theora | 3 | ffmpeg-full, openmsx, scummvm |
| talloc | 3 | freeradius-server, notmuch, samba |
| qtpositioning | 3 | gammaray, pyqt, qdmr |
| libraw | 3 | geeqie, imagemagick-full, siril |
| glfw | 3 | glslviewer, libgr, librealsense |
| hicolor-icon-theme | 3 | gnome-papers, homebank, nip4 |
| qt@5 | 3 | gnuradio, mgba, poppler-qt5 |
| docbook-xsl | 3 | gtk-doc, kdoctools, xmlto |
| dbus | 3 | gtk-gnutella, onedrive-cli, qtbase |
| miniupnpc | 3 | i2pd, ppsspp, transmission-cli |
| lmdb | 3 | knot-resolver, neomutt, samba |
| libimobiledevice-glue | 3 | libimobiledevice, libirecovery, libusbmuxd |
| tinyxml2 | 3 | msc-generator, urdfdom, uuu |
| libxmu | 3 | ngspice, rxvt-unicode, xeyes |
| vtk | 3 | pcl, rtabmap, visp |
| gstreamer | 3 | pdfpc, pianod, spice-gtk |
| net-snmp | 3 | php, sane-backends, syslog-ng |
| qtserialport | 3 | pyqt, qdmr, qtserialbus |
| isl | 2 | aarch64-elf-gcc, arm-none-eabi-gcc |
| libbs2b | 2 | audacious, qmmp |
| leveldb | 2 | brpc, librime |
| utf8proc | 2 | ccextractor, rom-tools |
| llvm@20 | 2 | chapel, ldc |
| sdl2_net | 2 | chocolate-doom, dosbox-staging |
| metis | 2 | colmap, damask-grid |
| libint | 2 | cp2k, votca |
| autoconf | 2 | crosstool-ng, php |
| spdlog | 2 | cryfs, gnuradio |
| argon2 | 2 | dar, php |
| wxwidgets@3.2 | 2 | diff-pdf, spatialite-gui |
| concurrencykit | 2 | dnsperf, fq |
| libslirp | 2 | dosbox-staging, dosbox-x |
| woff2 | 2 | dvisvgm, fontforge |
| libxmp | 2 | easyrpg-player, qmmp |
| inih | 2 | easyrpg-player, rsgain |
| qtcharts | 2 | ecflow-ui, pyqt |
| libgee | 2 | enter-tex, pdfpc |
| libgedit-gtksourceview | 2 | enter-tex, gedit |
| libgedit-amtk | 2 | enter-tex, gedit |
| libgedit-tepl | 2 | enter-tex, gedit |
| erlang | 2 | erlang-language-platform, rebar3 |
| libnet | 2 | ettercap, syslog-ng |
| vips | 2 | faircamp, nip4 |
| mupdf | 2 | fancy-cat, gowall |
| yyjson | 2 | fastfetch, siril |
| grpc | 2 | fastnetmon, syslog-ng |
| frei0r | 2 | ffmpeg-full, mlt |
| rubberband | 2 | ffmpeg-full, mlt |
| srt | 2 | ffmpeg-full, tsduck |
| libssh | 2 | ffmpeg-full, wireshark |
| aom | 2 | ffmpeg-full, libheif |
| libvidstab | 2 | ffmpeg-full, mlt |
| libxau | 2 | fricas, xorg-server |
| libsm | 2 | fricas, ngspice |
| libxdmcp | 2 | fricas, xorg-server |
| libice | 2 | fricas, ngspice |
| libcbor | 2 | fwupd, libfido2 |
| usb.ids | 2 | fwupd, libosinfo |
| gtkglext | 2 | gabedit, gwyddion |
| qtconnectivity | 2 | gammaray, pyqt |
| qt3d | 2 | gammaray, pyqt |
| qtscxml | 2 | gammaray, pyqt |
| qtwebchannel | 2 | gammaray, pyqt |
| qtwebengine | 2 | gammaray, pyqt |
| mbedtls@3 | 2 | gauche, librist |
| libiconv | 2 | git, neomutt |
| soapysdr | 2 | gnuradio, limesuite |
| pygobject3 | 2 | gnuradio, gplugin |
| terminal-notifier | 2 | gopass, mikutter |
| gnupg | 2 | gopass, qca |
| libsigc++@2 | 2 | gsmartcontrol, synfig |
| glibmm@2.66 | 2 | gsmartcontrol, synfig |
| docbook | 2 | gtk-doc, xmlto |
| gssdp | 2 | gupnp, gupnp-tools |
| gtksourceview4 | 2 | gupnp-tools, siril |
| liblqr | 2 | imagemagick-full, synfig |
| libmicrohttpd | 2 | librist, musikcube |
| cjson | 2 | librist, pcl |
| gdal | 2 | mapserver, osmcoastline |
| libepoxy | 2 | mgba, spice-gtk |
| game-music-emu | 2 | musikcube, qmmp |
| libev | 2 | musikcube, percona-xtrabackup |
| tokyo-cabinet | 2 | mutt, neomutt |
| capstone | 2 | open-ocd, rizin |
| pandoc | 2 | pandoc-crossref, pandoc-plot |
| nss | 2 | poppler-qt5, qca |
| molten-vk | 2 | ppsspp, vulkan-tools |
| pulseaudio | 2 | qmmp, shairport-sync |
| musepack | 2 | qmmp, scummvm |
| libxft | 2 | rxvt-unicode, xfig |
| libxlsxwriter | 2 | sc-im, spatialite-gui |

### Long Tail (1 dependent each): 294 deps

Each needed by only a single recipe. Lower priority for creation.

<details>
<summary>Click to expand full list</summary>

| Dep Name | Recipe |
|----------|--------|
| a52dec | scummvm |
| abook | lbdb |
| argtable3 | astroterm |
| aribb24 | ffmpeg-full |
| assimp | qtquick3d |
| astgen | joern |
| atkmm@2.28 | gsmartcontrol |
| automake | crosstool-ng |
| awscli | rosa-cli |
| beads | gastown |
| beagle | mrbayes |
| binutils | crosstool-ng |
| botan | qca |
| bstring | netatalk |
| cairomm@1.14 | gsmartcontrol |
| ceres-solver | colmap |
| certifi | gyb |
| cffi | notmuch |
| cgal | colmap |
| cln | ginac |
| collectd | freeradius-server |
| confuse | fwup |
| console_bridge | urdfdom |
| cpanminus | lanraragi |
| cppzmq | gnuradio |
| cracklib | netatalk |
| cunit | libiscsi |
| dbcsr | cp2k |
| diffutils | midnight-commander |
| discount | pdfpc |
| docker-machine | docker-machine-driver-vmware |
| dosfstools | mender-artifact |
| dotnet@9 | jackett |
| double-conversion | qtbase |
| dpkg | apt |
| editorconfig | gnome-builder |
| etcd | vitess |
| etl | synfig |
| exempi | gnome-papers |
| faiss | colmap |
| fcgi | mapserver |
| fdk-aac | darkice |
| ffms2 | siril |
| flex | crosstool-ng |
| fltk | limesuite |
| fltk@1.3 | tiger-vnc |
| fstrm | knot-resolver |
| g2o | rtabmap |
| gawk | crosstool-ng |
| gcab | msitools |
| gcc@13 | opencoarrays |
| git-delta | diffnav |
| glm | supertux |
| glpk | msc-generator |
| glslang | vulkan-tools |
| gmime | notmuch |
| go@1.24 | llgo |
| goocanvas | gpredict |
| gpac | ccextractor |
| gpgmepp | poppler-qt5 |
| gputils | sdcc |
| grep | crosstool-ng |
| gsasl | mailutils |
| gsettings-desktop-schemas | gedit |
| gtk-mac-integration | gedit |
| gtkdatabox | klavaro |
| gtkmm3 | gsmartcontrol |
| gumbo-parser | qttools |
| gupnp-av | gupnp-tools |
| hamlib | gpredict |
| healpix | siril |
| helm@3 | vcluster |
| help2man | fatsort |
| highway | jpeg-xl |
| i2c-tools | ddcutil |
| ic-wasm | icp-cli |
| iir1 | dosbox-staging |
| iniparser | netatalk |
| isa-l | fastp |
| ivykis | syslog-ng |
| jemalloc | chapel |
| jlog | fq |
| jsoncpp | drogon |
| jsonrpc-glib | gnome-builder |
| kapacitor | chronograf |
| karchive | kdoctools |
| khard | lbdb |
| kmod | ddcutil |
| knot | knot-resolver |
| krb5 | samba |
| libaec | hdf5-mpi |
| libapplewm | xorg-server |
| libass | ffmpeg-full |
| libb2 | qtbase |
| libcanberra | dissent |
| libcddb | qmmp |
| libcdio-paranoia | qmmp |
| libconfig | shairport-sync |
| libcss | felinks |
| libcue | audacious |
| libdaemon | shairport-sync |
| libdbi | syslog-ng |
| libdc1394 | visp |
| libde265 | libheif |
| libdeflate | fastp |
| libdex | gnome-builder |
| libdom | felinks |
| libdrm | ddcutil |
| libdv | mlt |
| libdvdcss | lsdvd |
| libdvdread | lsdvd |
| libebur128 | rsgain |
| libecpint | votca |
| libetpan | claws-mail |
| libffcall | clisp |
| libfixposix | jruby |
| libfuse@2 | cryfs |
| libgedit-gfls | gedit |
| libgit2-glib | gnome-builder |
| libid3tag | audiowaveform |
| libjcat | fwupd |
| libjodycode | jdupes |
| liblcf | easyrpg-player |
| libmms | qmmp |
| libmng | synfig |
| libmodbus | mbpoll |
| libmpdclient | ncmpcpp |
| libmpeg2 | scummvm |
| libngspice | ngspice |
| libofx | homebank |
| libpanel | gnome-builder |
| libpaper | a2ps |
| libpeas | gnome-builder |
| libpeas@1 | gedit |
| libpg_query | postgres-language-server |
| libpipeline | man-db |
| libplacebo | ffmpeg-full |
| libpqxx | osm2pgrouting |
| libptytty | rlwrap |
| librdkafka | syslog-ng |
| libre | baresip |
| libsecret | git-credential-libsecret |
| libshout | qmmp |
| libsidplayfp | audacious |
| libsigrok | sigrok-cli |
| libsigrokdecode | sigrok-cli |
| libsigsegv | clisp |
| libsmi | wireshark |
| libsolv | micromamba |
| libspectre | pqiv |
| libspiro | fontforge |
| libtatsu | libimobiledevice |
| libtorrent-rakshasa | rtorrent |
| libtpms | swtpm |
| libudfread | libbluray |
| libultrahdr | imagemagick-full |
| libuninameslist | fontforge |
| libvatek | tsduck |
| libvirt | terraform-provider-libvirt |
| libvncserver | x11vnc |
| libwapcaplet | felinks |
| libwebsockets | ttyd |
| libxaw | ngspice |
| libxaw3d | xfig |
| libxcrypt | samba |
| libxfixes | xorg-server |
| libxfont2 | xorg-server |
| libxi | xeyes |
| libxinerama | feh |
| libxls | sc-im |
| libxml++ | synfig |
| libxrandr | ddcutil |
| liquid-dsp | inspectrum |
| litehtml | qttools |
| llama.cpp | ffmpeg-full |
| lld@19 | llgo |
| lld@21 | c3c |
| llvm@19 | llgo |
| lmfit | gromacs |
| log4cpp | fastnetmon |
| lpeg | corsixth |
| lzip | crosstool-ng |
| lzlib | wget2 |
| m4 | crosstool-ng |
| magic_enum | thors-anvil |
| marisa | librime |
| maxima | wxmaxima |
| md4c | qtbase |
| mesa | xorg-server |
| mpdecimal | python-freethreading |
| mplayer | qmmp |
| msgpack | neovim-qt |
| mt32emu | dosbox-staging |
| mtools | mender-artifact |
| mujs | fancy-cat |
| muparser | gromacs |
| neovim | neovim-qt |
| netcdf | netcdf-fortran |
| nim | min-lang |
| node@22 | opensearch-dashboards |
| node@24 | zeek |
| ocaml | rocq |
| ocaml-findlib | rocq |
| ocaml-zarith | rocq |
| octave | dynare |
| octomap | rtabmap |
| opencc | librime |
| opencore-amr | ffmpeg-full |
| openimageio | colmap |
| openjdk@11 | cfr-decompiler |
| osinfo-db | libosinfo |
| pangomm@2.46 | gsmartcontrol |
| pdal | rtabmap |
| perl-dbd-mysql | percona-toolkit |
| petsc | damask-grid |
| pgrouting | osm2pgrouting |
| phodav | spice-gtk |
| php@8.4 | phpbrew |
| pillow | bazarr |
| pkcs11-helper | qca |
| plotutils | pstoedit |
| pmix | open-mpi |
| poselib | colmap |
| postgis | osm2pgrouting |
| potrace | dvisvgm |
| projectm | qmmp |
| protobuf@33 | brpc |
| psutils | groff |
| pyenv | pyenv-virtualenv |
| pyqt@5 | gnuradio |
| qemu | macpine |
| qtdatavis3d | pyqt |
| qtimageformats | audacious |
| qtlocation | gammaray |
| qtnetworkauth | pyqt |
| qtquicktimeline | qtquick3d |
| qtremoteobjects | pyqt |
| qtsensors | pyqt |
| qtspeech | pyqt |
| qtwebsockets | pyqt |
| qwt-qt5 | gnuradio |
| readosm | spatialite-tools |
| reproc | micromamba |
| riemann-client | syslog-ng |
| rpds-py | gnuradio |
| sdl3 | rom-tools |
| sequoia-sqv | apt |
| sfsexp | notmuch |
| shared-mime-info | libheif |
| simdjson | micromamba |
| slicot | dynare |
| soapyrtlsdr | gnuradio |
| sox | mlt |
| spice-protocol | spice-gtk |
| tass64 | prog8 |
| tbb | open-image-denoise |
| tcl-tk@8 | x3270 |
| tdb | samba |
| template-glib | gnome-builder |
| tesseract | ffmpeg-full |
| tevent | samba |
| texlive | dvisvgm |
| tre | felinks |
| two-lame | darkice |
| uchardet | groff |
| uhd | gnuradio |
| unbound | gnutls |
| urdfdom_headers | urdfdom |
| util-macros | makedepend |
| virtualpg | spatialite-gui |
| volk | gnuradio |
| vte3 | gnome-builder |
| vulkan-headers | vulkan-tools |
| wandio | libtrace |
| wget | mmseqs2 |
| whisper-cpp | ffmpeg-full |
| wildmidi | qmmp |
| xapian | notmuch |
| xauth | xorg-server |
| xcb-util | xorg-server |
| xcb-util-image | xorg-server |
| xcb-util-keysyms | xorg-server |
| xcb-util-renderutil | xorg-server |
| xcb-util-wm | xorg-server |
| xkbcomp | xorg-server |
| xkeyboard-config | xorg-server |
| xmlrpc-c | rtorrent |
| xorgproto | makedepend |
| xvid | ffmpeg-full |
| yamale | chart-testing |
| yuicompressor | emscripten |
| z3 | wuppiefuzz |
| zbar | visp |
| zimg | ffmpeg-full |

</details>

## Recipes That Can Be Backfilled Immediately

These 13 recipes have missing deps where every dep already has a recipe.
Only requires adding the dep name to `runtime_dependencies` in the TOML.

| Recipe | Formula | Current Deps | Add | Correct Value |
|--------|---------|-------------|-----|---------------|
| coin3d | coin3d | (none) | qtbase | qtbase |
| ctags-lsp | ctags-lsp | (none) | universal-ctags | universal-ctags |
| dump1090-fa | dump1090-fa | ncurses | libbladerf, librtlsdr | libbladerf, librtlsdr, ncurses |
| ffmpegthumbnailer | ffmpegthumbnailer | jpeg-turbo, libpng | ffmpeg | ffmpeg, jpeg-turbo, libpng |
| fontconfig | fontconfig | (none) | freetype, gettext | freetype, gettext |
| get-iplayer | get_iplayer | atomicparsley | ffmpeg | atomicparsley, ffmpeg |
| glib | glib | (none) | gettext, pcre2 | gettext, pcre2 |
| libgit2 | libgit2 | (none) | libssh2 | libssh2 |
| libxml2 | libxml2 | (none) | readline | readline |
| ocicl | ocicl | zstd | sbcl | sbcl, zstd |
| open-simh | open-simh | libpng | vde | libpng, vde |
| pngcrush | pngcrush | (none) | libpng | libpng |
| sqlite | sqlite | (none) | readline | readline |

## Existing Dep Recipes Needing Backfill

66 deps that already have recipes in the registry but aren't referenced
by tool recipes that need them. These represent 126 recipe-dep pairs.

| Dep Recipe | Count | Recipes Needing It |
|------------|-------|-------------------|
| ffmpeg | 18 | audacious, bazarr, corsixth, faircamp, fceux, ffmpegthumbnailer, get-iplayer, glslviewer, libgr, mgba, mlt, musikcube, navidrome, pc6001vx, pianobar, qmmp, rsgain, siril |
| qtbase | 14 | audacious, coin3d, colmap, ecflow-ui, fceux, gammaray, gwenhywfar, inspectrum, kdoctools, libgr, mlt, neovim-qt, pc6001vx, pcl |
| libsndfile | 6 | audacious, audiowaveform, easyrpg-player, fluid-synth, frotz, gnuradio |
| gettext | 6 | cairo, fontconfig, git, glib, libidn2, notmuch |
| protobuf-c | 4 | ccextractor, fwupd, knot-resolver, mapserver |
| x265 | 3 | fceux, ffmpeg-full, libheif |
| tcl-tk | 3 | gensio, openmsx, sqlite-analyzer |
| libpng | 2 | cairo, pngcrush |
| freetype | 2 | cairo, fontconfig |
| glib | 2 | cairo, notmuch |
| suite-sparse | 2 | colmap, dynare |
| open-mpi | 2 | cp2k, damask-grid |
| unixodbc | 2 | edbrowse, freetds |
| jpeg-xl | 2 | ffmpeg-full, geeqie |
| sbcl | 2 | fricas, ocicl |
| json-glib | 2 | fwupd, gnome-builder |
| pcre2 | 2 | git, glib |
| libimobiledevice | 2 | ideviceinstaller, ios-webkit-debug-proxy |
| libssh2 | 2 | libcurl, libgit2 |
| readline | 2 | libxml2, sqlite |
| libopenmpt | 1 | audacious |
| libusb-compat | 1 | avrdude |
| fontconfig | 1 | cairo |
| libxc | 1 | cp2k |
| universal-ctags | 1 | ctags-lsp |
| hdf5-mpi | 1 | damask-grid |
| libgpg-error | 1 | dar |
| libgcrypt | 1 | dar |
| ldns | 1 | dnsperf |
| libbladerf | 1 | dump1090-fa |
| librtlsdr | 1 | dump1090-fa |
| libmatio | 1 | dynare |
| rebar3 | 1 | erlang-language-platform |
| mongo-c-driver | 1 | fastnetmon |
| imlib2 | 1 | feh |
| librist | 1 | ffmpeg-full |
| libvmaf | 1 | ffmpeg-full |
| svt-av1 | 1 | ffmpeg-full |
| libbluray | 1 | ffmpeg-full |
| speex | 1 | ffmpeg-full |
| innoextract | 1 | fheroes2 |
| libxpm | 1 | fricas |
| libxmlb | 1 | fwupd |
| qtdeclarative | 1 | gammaray |
| qttools | 1 | gammaray |
| little-cms2 | 1 | geeqie |
| libheif | 1 | geeqie |
| p11-kit | 1 | gnutls |
| libtasn1 | 1 | gnutls |
| php | 1 | joern |
| libnghttp2 | 1 | libcurl |
| brotli | 1 | libcurl |
| libnghttp3 | 1 | libcurl |
| libngtcp2 | 1 | libcurl |
| zstd | 1 | libcurl |
| s-lang | 1 | midnight-commander |
| srecord | 1 | minipro |
| vde | 1 | open-simh |
| swift-protobuf | 1 | protoc-gen-grpc-swift |
| wxwidgets | 1 | scummvm-tools |
| geos | 1 | spatialite |
| libxml2 | 1 | spatialite |
| sqlite | 1 | spatialite |
| proj | 1 | spatialite |
| usbredir | 1 | spice-gtk |
| xz | 1 | zstd |

## Full Per-Recipe Backfill Specification

For every affected recipe, the exact `runtime_dependencies` per the Homebrew
API. Deps marked with `(*)` do not have a recipe in the registry yet.

### a2ps

- **Formula**: `a2ps`
- **Current**: `["bdw-gc"]`
- **Full correct**: `[bdw-gc, libpaper (*)]`
- **Blocked** (no recipe): libpaper

### aarch64-elf-gcc

- **Formula**: `aarch64-elf-gcc`
- **Current**: `["aarch64-elf-binutils", "gmp", "zstd"]`
- **Full correct**: `[aarch64-elf-binutils, gmp, isl (*), libmpc (*), mpfr (*), zstd]`
- **Blocked** (no recipe): isl, libmpc, mpfr

### aarch64-elf-gdb

- **Formula**: `aarch64-elf-gdb`
- **Current**: `["gmp", "ncurses", "readline", "xz", "zstd"]`
- **Full correct**: `[gmp, mpfr (*), ncurses, python@3.14 (*), readline, xz, zstd]`
- **Blocked** (no recipe): mpfr, python@3.14

### ada-url

- **Formula**: `ada-url`
- **Current**: `[]`
- **Full correct**: `[fmt (*)]`
- **Blocked** (no recipe): fmt

### afflib

- **Formula**: `afflib`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### aircrack-ng

- **Formula**: `aircrack-ng`
- **Current**: `["pcre2", "sqlite"]`
- **Full correct**: `[openssl@3 (*), pcre2, sqlite]`
- **Blocked** (no recipe): openssl@3

### alda

- **Formula**: `alda`
- **Current**: `[]`
- **Full correct**: `[openjdk (*)]`
- **Blocked** (no recipe): openjdk

### allureofthestars

- **Formula**: `allureofthestars`
- **Current**: `["gmp", "sdl2"]`
- **Full correct**: `[gmp, sdl2, sdl2_ttf (*)]`
- **Blocked** (no recipe): sdl2_ttf

### aoe

- **Formula**: `aoe`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*), tmux (*)]`
- **Blocked** (no recipe): openssl@3, tmux

### apache-polaris

- **Formula**: `apache-polaris`
- **Current**: `[]`
- **Full correct**: `[openjdk (*)]`
- **Blocked** (no recipe): openjdk

### apr-util

- **Formula**: `apr-util`
- **Current**: `["apr", "openssl"]`
- **Full correct**: `[apr, openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### apt

- **Formula**: `apt`
- **Current**: `["bzip2", "perl", "xz", "zstd"]`
- **Full correct**: `[berkeley-db@5 (*), bzip2, dpkg (*), gcc (*), lz4 (*), openssl@3 (*), perl, sequoia-sqv (*), systemd (*), xxhash (*), xz, zlib-ng-compat (*), zstd]`
- **Blocked** (no recipe): berkeley-db@5, dpkg, gcc, lz4, openssl@3, sequoia-sqv, systemd, xxhash, zlib-ng-compat

### argyll-cms

- **Formula**: `argyll-cms`
- **Current**: `["jpeg-turbo", "libpng"]`
- **Full correct**: `[jpeg-turbo, libpng, libtiff (*), openssl@3 (*)]`
- **Blocked** (no recipe): libtiff, openssl@3

### arm-none-eabi-gcc

- **Formula**: `arm-none-eabi-gcc`
- **Current**: `["arm-none-eabi-binutils", "gmp", "zstd"]`
- **Full correct**: `[arm-none-eabi-binutils, gmp, isl (*), libmpc (*), mpfr (*), zstd]`
- **Blocked** (no recipe): isl, libmpc, mpfr

### arm-none-eabi-gdb

- **Formula**: `arm-none-eabi-gdb`
- **Current**: `["gmp", "ncurses", "readline", "xz", "zstd"]`
- **Full correct**: `[gmp, mpfr (*), ncurses, python@3.14 (*), readline, xz, zstd]`
- **Blocked** (no recipe): mpfr, python@3.14

### astrometry-net

- **Formula**: `astrometry-net`
- **Current**: `["cairo", "cfitsio", "gsl", "jpeg-turbo", "libpng", "numpy"]`
- **Full correct**: `[cairo, cfitsio, gsl, jpeg-turbo, libpng, netpbm (*), numpy, python@3.14 (*), wcslib (*)]`
- **Blocked** (no recipe): netpbm, python@3.14, wcslib

### astroterm

- **Formula**: `astroterm`
- **Current**: `[]`
- **Full correct**: `[argtable3 (*)]`
- **Blocked** (no recipe): argtable3

### asymptote

- **Formula**: `asymptote`
- **Current**: `["bdw-gc", "gsl", "readline"]`
- **Full correct**: `[bdw-gc, fftw (*), ghostscript (*), gsl, readline]`
- **Blocked** (no recipe): fftw, ghostscript

### audacious

- **Formula**: `audacious`
- **Current**: `["faad2", "fluid-synth", "gettext", "glib", "libnotify", "neon", "sdl2"]`
- **Full correct**: `[faad2, ffmpeg, flac (*), fluid-synth, gdk-pixbuf (*), gettext, glib, lame (*), libbs2b (*), libcue (*), libmodplug (*), libnotify, libogg (*), libopenmpt, libsamplerate (*), libsidplayfp (*), libsndfile, libsoxr (*), libvorbis (*), mpg123 (*), neon, opusfile (*), qtbase, qtimageformats (*), qtmultimedia (*), qtsvg (*), sdl2, wavpack (*)]`
- **Can add now**: ffmpeg, libopenmpt, libsndfile, qtbase
- **Blocked** (no recipe): flac, gdk-pixbuf, lame, libbs2b, libcue, libmodplug, libogg, libsamplerate, libsidplayfp, libsoxr, libvorbis, mpg123, opusfile, qtimageformats, qtmultimedia, qtsvg, wavpack

### audiowaveform

- **Formula**: `audiowaveform`
- **Current**: `[]`
- **Full correct**: `[boost (*), gd (*), libid3tag (*), libsndfile, mad (*)]`
- **Can add now**: libsndfile
- **Blocked** (no recipe): boost, gd, libid3tag, mad

### avrdude

- **Formula**: `avrdude`
- **Current**: `["hidapi"]`
- **Full correct**: `[hidapi, libftdi (*), libusb (*), libusb-compat]`
- **Can add now**: libusb-compat
- **Blocked** (no recipe): libftdi, libusb

### avro-c

- **Formula**: `avro-c`
- **Current**: `["snappy", "xz"]`
- **Full correct**: `[jansson (*), snappy, xz]`
- **Blocked** (no recipe): jansson

### avro-cpp

- **Formula**: `avro-cpp`
- **Current**: `["zstd"]`
- **Full correct**: `[boost (*), zstd]`
- **Blocked** (no recipe): boost

### bareos-client

- **Formula**: `bareos-client`
- **Current**: `["gettext", "readline"]`
- **Full correct**: `[gettext, jansson (*), lzo (*), openssl@3 (*), readline, xxhash (*)]`
- **Blocked** (no recipe): jansson, lzo, openssl@3, xxhash

### baresip

- **Formula**: `baresip`
- **Current**: `[]`
- **Full correct**: `[libre (*), openssl@3 (*)]`
- **Blocked** (no recipe): libre, openssl@3

### bazarr

- **Formula**: `bazarr`
- **Current**: `["numpy", "unar"]`
- **Full correct**: `[ffmpeg, numpy, pillow (*), python@3.14 (*), unar]`
- **Can add now**: ffmpeg
- **Blocked** (no recipe): pillow, python@3.14

### bazel

- **Formula**: `bazel`
- **Current**: `[]`
- **Full correct**: `[openjdk@21 (*)]`
- **Blocked** (no recipe): openjdk@21

### bbtools

- **Formula**: `bbtools`
- **Current**: `[]`
- **Full correct**: `[openjdk (*)]`
- **Blocked** (no recipe): openjdk

### bcftools

- **Formula**: `bcftools`
- **Current**: `["gsl"]`
- **Full correct**: `[gsl, htslib (*)]`
- **Blocked** (no recipe): htslib

### bedops

- **Formula**: `bedops`
- **Current**: `[]`
- **Full correct**: `[jansson (*)]`
- **Blocked** (no recipe): jansson

### bento4

- **Formula**: `bento4`
- **Current**: `[]`
- **Full correct**: `[python@3.14 (*)]`
- **Blocked** (no recipe): python@3.14

### berkeley-db

- **Formula**: `berkeley-db`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### biber

- **Formula**: `biber`
- **Current**: `["perl"]`
- **Full correct**: `[openssl@3 (*), perl]`
- **Blocked** (no recipe): openssl@3

### bigloo

- **Formula**: `bigloo`
- **Current**: `["bdw-gc", "gmp", "pcre2", "sqlite"]`
- **Full correct**: `[bdw-gc, gmp, libunistring (*), libuv (*), openjdk (*), openssl@3 (*), pcre2, sqlite]`
- **Blocked** (no recipe): libunistring, libuv, openjdk, openssl@3

### bochs

- **Formula**: `bochs`
- **Current**: `["sdl2"]`
- **Full correct**: `[libtool (*), sdl2]`
- **Blocked** (no recipe): libtool

### brpc

- **Formula**: `brpc`
- **Current**: `["abseil"]`
- **Full correct**: `[abseil, gflags (*), leveldb (*), openssl@3 (*), protobuf@33 (*)]`
- **Blocked** (no recipe): gflags, leveldb, openssl@3, protobuf@33

### c3c

- **Formula**: `c3c`
- **Current**: `[]`
- **Full correct**: `[lld@21 (*), llvm@21 (*)]`
- **Blocked** (no recipe): lld@21, llvm@21

### cairo

- **Formula**: `cairo`
- **Current**: `[]`
- **Full correct**: `[fontconfig, freetype, gettext, glib, libpng, libx11 (*), libxcb (*), libxext (*), libxrender (*), lzo (*), pixman (*)]`
- **Can add now**: fontconfig, freetype, gettext, glib, libpng
- **Blocked** (no recipe): libx11, libxcb, libxext, libxrender, lzo, pixman

### ccache

- **Formula**: `ccache`
- **Current**: `["blake3", "zstd"]`
- **Full correct**: `[blake3, fmt (*), hiredis (*), xxhash (*), zstd]`
- **Blocked** (no recipe): fmt, hiredis, xxhash

### ccextractor

- **Formula**: `ccextractor`
- **Current**: `["freetype", "libpng"]`
- **Full correct**: `[freetype, gpac (*), libpng, protobuf-c, utf8proc (*)]`
- **Can add now**: protobuf-c
- **Blocked** (no recipe): gpac, utf8proc

### ccls

- **Formula**: `ccls`
- **Current**: `[]`
- **Full correct**: `[llvm (*)]`
- **Blocked** (no recipe): llvm

### cdogs-sdl

- **Formula**: `cdogs-sdl`
- **Current**: `["sdl2"]`
- **Full correct**: `[python@3.14 (*), sdl2, sdl2_image (*), sdl2_mixer (*)]`
- **Blocked** (no recipe): python@3.14, sdl2_image, sdl2_mixer

### cdrdao

- **Formula**: `cdrdao`
- **Current**: `[]`
- **Full correct**: `[lame (*), libao (*), libvorbis (*), mad (*)]`
- **Blocked** (no recipe): lame, libao, libvorbis, mad

### cdxgen

- **Formula**: `cdxgen`
- **Current**: `["ruby", "sourcekitten", "sqlite", "trivy"]`
- **Full correct**: `[dotnet (*), node (*), ruby, sourcekitten, sqlite, trivy]`
- **Blocked** (no recipe): dotnet, node

### cfr-decompiler

- **Formula**: `cfr-decompiler`
- **Current**: `[]`
- **Full correct**: `[openjdk@11 (*)]`
- **Blocked** (no recipe): openjdk@11

### cgns

- **Formula**: `cgns`
- **Current**: `[]`
- **Full correct**: `[gcc (*), hdf5 (*)]`
- **Blocked** (no recipe): gcc, hdf5

### chapel

- **Formula**: `chapel`
- **Current**: `["cmake", "gmp", "pkgconf"]`
- **Full correct**: `[cmake, gmp, hwloc (*), jemalloc (*), llvm@20 (*), pkgconf, python@3.14 (*)]`
- **Blocked** (no recipe): hwloc, jemalloc, llvm@20, python@3.14

### chart-testing

- **Formula**: `chart-testing`
- **Current**: `[]`
- **Full correct**: `[yamale (*)]`
- **Blocked** (no recipe): yamale

### chocolate-doom

- **Formula**: `chocolate-doom`
- **Current**: `["fluid-synth", "libpng", "sdl2"]`
- **Full correct**: `[fluid-synth, libpng, libsamplerate (*), sdl2, sdl2_mixer (*), sdl2_net (*)]`
- **Blocked** (no recipe): libsamplerate, sdl2_mixer, sdl2_net

### chronograf

- **Formula**: `chronograf`
- **Current**: `["influxdb"]`
- **Full correct**: `[influxdb, kapacitor (*)]`
- **Blocked** (no recipe): kapacitor

### chrony

- **Formula**: `chrony`
- **Current**: `["gnutls"]`
- **Full correct**: `[gnutls, nettle (*)]`
- **Blocked** (no recipe): nettle

### clang-uml

- **Formula**: `clang-uml`
- **Current**: `[]`
- **Full correct**: `[llvm@21 (*), yaml-cpp (*)]`
- **Blocked** (no recipe): llvm@21, yaml-cpp

### claws-mail

- **Formula**: `claws-mail`
- **Current**: `["cairo", "gettext", "glib", "gnutls"]`
- **Full correct**: `[at-spi2-core (*), cairo, gdk-pixbuf (*), gettext, glib, gnutls, gtk+3 (*), harfbuzz (*), libetpan (*), nettle (*), pango (*)]`
- **Blocked** (no recipe): at-spi2-core, gdk-pixbuf, gtk+3, harfbuzz, libetpan, nettle, pango

### clazy

- **Formula**: `clazy`
- **Current**: `[]`
- **Full correct**: `[coreutils (*), llvm (*)]`
- **Blocked** (no recipe): coreutils, llvm

### clifm

- **Formula**: `clifm`
- **Current**: `["gettext", "readline"]`
- **Full correct**: `[gettext, libmagic (*), readline]`
- **Blocked** (no recipe): libmagic

### clisp

- **Formula**: `clisp`
- **Current**: `["readline"]`
- **Full correct**: `[libffcall (*), libsigsegv (*), readline]`
- **Blocked** (no recipe): libffcall, libsigsegv

### cljfmt

- **Formula**: `cljfmt`
- **Current**: `[]`
- **Full correct**: `[openjdk (*)]`
- **Blocked** (no recipe): openjdk

### code-cli

- **Formula**: `code-cli`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### codex-acp

- **Formula**: `codex-acp`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### coin3d

- **Formula**: `coin3d`
- **Current**: `[]`
- **Full correct**: `[qtbase]`
- **Can add now**: qtbase

### colmap

- **Formula**: `colmap`
- **Current**: `["glew", "gmp", "sqlite"]`
- **Full correct**: `[boost (*), ceres-solver (*), cgal (*), eigen (*), faiss (*), flann (*), gflags (*), glew, glog (*), gmp, libomp (*), lz4 (*), metis (*), mpfr (*), openimageio (*), openssl@3 (*), poselib (*), qtbase, sqlite, suite-sparse]`
- **Can add now**: qtbase, suite-sparse
- **Blocked** (no recipe): boost, ceres-solver, cgal, eigen, faiss, flann, gflags, glog, libomp, lz4, metis, mpfr, openimageio, openssl@3, poselib

### core-lightning

- **Formula**: `core-lightning`
- **Current**: `["bitcoin"]`
- **Full correct**: `[bitcoin, libsodium (*)]`
- **Blocked** (no recipe): libsodium

### corsixth

- **Formula**: `corsixth`
- **Current**: `["freetype", "lua", "sdl2"]`
- **Full correct**: `[ffmpeg, freetype, lpeg (*), lua, sdl2, sdl2_mixer (*)]`
- **Can add now**: ffmpeg
- **Blocked** (no recipe): lpeg, sdl2_mixer

### coturn

- **Formula**: `coturn`
- **Current**: `["libevent"]`
- **Full correct**: `[hiredis (*), libevent, libpq (*), openssl@3 (*)]`
- **Blocked** (no recipe): hiredis, libpq, openssl@3

### cp2k

- **Formula**: `cp2k`
- **Current**: `[]`
- **Full correct**: `[dbcsr (*), fftw (*), gcc (*), libint (*), libomp (*), libxc, open-mpi, openblas (*), scalapack (*)]`
- **Can add now**: libxc, open-mpi
- **Blocked** (no recipe): dbcsr, fftw, gcc, libint, libomp, openblas, scalapack

### crosstool-ng

- **Formula**: `crosstool-ng`
- **Current**: `["bash", "bison", "gettext", "gnu-sed", "make", "ncurses", "xz"]`
- **Full correct**: `[autoconf (*), automake (*), bash, binutils (*), bison, coreutils (*), flex (*), gawk (*), gettext, gnu-sed, grep (*), libtool (*), lzip (*), m4 (*), make, ncurses, python@3.14 (*), xz]`
- **Blocked** (no recipe): autoconf, automake, binutils, coreutils, flex, gawk, grep, libtool, lzip, m4, python@3.14

### cryfs

- **Formula**: `cryfs`
- **Current**: `[]`
- **Full correct**: `[boost (*), fmt (*), libfuse@2 (*), spdlog (*)]`
- **Blocked** (no recipe): boost, fmt, libfuse@2, spdlog

### ctags-lsp

- **Formula**: `ctags-lsp`
- **Current**: `[]`
- **Full correct**: `[universal-ctags]`
- **Can add now**: universal-ctags

### damask-grid

- **Formula**: `damask-grid`
- **Current**: `[]`
- **Full correct**: `[fftw (*), gcc (*), hdf5-mpi, libomp (*), metis (*), open-mpi, openblas (*), petsc (*), scalapack (*)]`
- **Can add now**: hdf5-mpi, open-mpi
- **Blocked** (no recipe): fftw, gcc, libomp, metis, openblas, petsc, scalapack

### dar

- **Formula**: `dar`
- **Current**: `["gettext", "xz", "zstd"]`
- **Full correct**: `[argon2 (*), gettext, libgcrypt, libgpg-error, lz4 (*), lzo (*), xz, zstd]`
- **Can add now**: libgcrypt, libgpg-error
- **Blocked** (no recipe): argon2, lz4, lzo

### darkice

- **Formula**: `darkice`
- **Current**: `["faac", "jack"]`
- **Full correct**: `[faac, fdk-aac (*), jack, lame (*), libogg (*), libsamplerate (*), libvorbis (*), two-lame (*)]`
- **Blocked** (no recipe): fdk-aac, lame, libogg, libsamplerate, libvorbis, two-lame

### dcmtk

- **Formula**: `dcmtk`
- **Current**: `["jpeg-turbo", "libpng"]`
- **Full correct**: `[jpeg-turbo, libpng, libtiff (*), openssl@3 (*)]`
- **Blocked** (no recipe): libtiff, openssl@3

### ddcutil

- **Formula**: `ddcutil`
- **Current**: `["glib"]`
- **Full correct**: `[glib, i2c-tools (*), jansson (*), kmod (*), libdrm (*), libusb (*), libx11 (*), libxext (*), libxrandr (*), systemd (*)]`
- **Blocked** (no recipe): i2c-tools, jansson, kmod, libdrm, libusb, libx11, libxext, libxrandr, systemd

### dfu-programmer

- **Formula**: `dfu-programmer`
- **Current**: `[]`
- **Full correct**: `[libusb (*)]`
- **Blocked** (no recipe): libusb

### dfu-util

- **Formula**: `dfu-util`
- **Current**: `[]`
- **Full correct**: `[libusb (*)]`
- **Blocked** (no recipe): libusb

### diff-pdf

- **Formula**: `diff-pdf`
- **Current**: `["cairo", "gettext", "glib"]`
- **Full correct**: `[cairo, gettext, glib, poppler (*), wxwidgets@3.2 (*)]`
- **Blocked** (no recipe): poppler, wxwidgets@3.2

### diffnav

- **Formula**: `diffnav`
- **Current**: `[]`
- **Full correct**: `[git-delta (*)]`
- **Blocked** (no recipe): git-delta

### dissent

- **Formula**: `dissent`
- **Current**: `["cairo", "gettext", "glib"]`
- **Full correct**: `[cairo, gdk-pixbuf (*), gettext, glib, gobject-introspection (*), graphene (*), gtk4 (*), gtksourceview5 (*), harfbuzz (*), libadwaita (*), libcanberra (*), libspelling (*), pango (*)]`
- **Blocked** (no recipe): gdk-pixbuf, gobject-introspection, graphene, gtk4, gtksourceview5, harfbuzz, libadwaita, libcanberra, libspelling, pango

### distcc

- **Formula**: `distcc`
- **Current**: `[]`
- **Full correct**: `[python@3.14 (*)]`
- **Blocked** (no recipe): python@3.14

### djvulibre

- **Formula**: `djvulibre`
- **Current**: `["jpeg-turbo"]`
- **Full correct**: `[jpeg-turbo, libtiff (*)]`
- **Blocked** (no recipe): libtiff

### dnsperf

- **Formula**: `dnsperf`
- **Current**: `["libnghttp2"]`
- **Full correct**: `[concurrencykit (*), ldns, libnghttp2, openssl@3 (*)]`
- **Can add now**: ldns
- **Blocked** (no recipe): concurrencykit, openssl@3

### docker-machine-driver-vmware

- **Formula**: `docker-machine-driver-vmware`
- **Current**: `[]`
- **Full correct**: `[docker-machine (*)]`
- **Blocked** (no recipe): docker-machine

### dockerfilegraph

- **Formula**: `dockerfilegraph`
- **Current**: `[]`
- **Full correct**: `[graphviz (*)]`
- **Blocked** (no recipe): graphviz

### doltgres

- **Formula**: `doltgres`
- **Current**: `[]`
- **Full correct**: `[icu4c@78 (*)]`
- **Blocked** (no recipe): icu4c@78

### dosbox-staging

- **Formula**: `dosbox-staging`
- **Current**: `["fluid-synth", "glib", "libpng", "sdl2"]`
- **Full correct**: `[fluid-synth, glib, iir1 (*), libpng, libslirp (*), mt32emu (*), opusfile (*), sdl2, sdl2_image (*), sdl2_net (*), speexdsp (*)]`
- **Blocked** (no recipe): iir1, libslirp, mt32emu, opusfile, sdl2_image, sdl2_net, speexdsp

### dosbox-x

- **Formula**: `dosbox-x`
- **Current**: `["fluid-synth", "freetype", "gettext", "glib", "libpng", "sdl2"]`
- **Full correct**: `[fluid-synth, freetype, gettext, glib, libpng, libslirp (*), sdl2]`
- **Blocked** (no recipe): libslirp

### dovecot

- **Formula**: `dovecot`
- **Current**: `[]`
- **Full correct**: `[openldap (*), openssl@3 (*)]`
- **Blocked** (no recipe): openldap, openssl@3

### drogon

- **Formula**: `drogon`
- **Current**: `["brotli", "c-ares"]`
- **Full correct**: `[brotli, c-ares, jsoncpp (*), openssl@3 (*)]`
- **Blocked** (no recipe): jsoncpp, openssl@3

### dump1090-fa

- **Formula**: `dump1090-fa`
- **Current**: `["ncurses"]`
- **Full correct**: `[libbladerf, librtlsdr, ncurses]`
- **Can add now**: libbladerf, librtlsdr

### dvisvgm

- **Formula**: `dvisvgm`
- **Current**: `["brotli", "freetype"]`
- **Full correct**: `[brotli, freetype, ghostscript (*), potrace (*), texlive (*), woff2 (*)]`
- **Blocked** (no recipe): ghostscript, potrace, texlive, woff2

### dynare

- **Formula**: `dynare`
- **Current**: `["gsl"]`
- **Full correct**: `[gcc (*), gsl, libmatio, libomp (*), octave (*), openblas (*), slicot (*), suite-sparse]`
- **Can add now**: libmatio, suite-sparse
- **Blocked** (no recipe): gcc, libomp, octave, openblas, slicot

### easyrpg-player

- **Formula**: `easyrpg-player`
- **Current**: `["freetype", "libpng", "sdl2"]`
- **Full correct**: `[fmt (*), freetype, harfbuzz (*), icu4c@78 (*), inih (*), liblcf (*), libogg (*), libpng, libsndfile, libvorbis (*), libxmp (*), mpg123 (*), pixman (*), sdl2, speexdsp (*)]`
- **Can add now**: libsndfile
- **Blocked** (no recipe): fmt, harfbuzz, icu4c@78, inih, liblcf, libogg, libvorbis, libxmp, mpg123, pixman, speexdsp

### ecflow-ui

- **Formula**: `ecflow-ui`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*), qt5compat (*), qtbase, qtcharts (*), qtsvg (*)]`
- **Can add now**: qtbase
- **Blocked** (no recipe): openssl@3, qt5compat, qtcharts, qtsvg

### edbrowse

- **Formula**: `edbrowse`
- **Current**: `["curl", "pcre2", "readline"]`
- **Full correct**: `[curl, openssl@3 (*), pcre2, readline, unixodbc]`
- **Can add now**: unixodbc
- **Blocked** (no recipe): openssl@3

### eiffelstudio

- **Formula**: `eiffelstudio`
- **Current**: `["cairo", "gettext", "glib"]`
- **Full correct**: `[at-spi2-core (*), cairo, gdk-pixbuf (*), gettext, glib, gtk+3 (*), harfbuzz (*), libx11 (*), pango (*)]`
- **Blocked** (no recipe): at-spi2-core, gdk-pixbuf, gtk+3, harfbuzz, libx11, pango

### elan-init

- **Formula**: `elan-init`
- **Current**: `["gmp"]`
- **Full correct**: `[coreutils (*), gmp]`
- **Blocked** (no recipe): coreutils

### emscripten

- **Formula**: `emscripten`
- **Current**: `[]`
- **Full correct**: `[node (*), openjdk (*), python@3.14 (*), yuicompressor (*)]`
- **Blocked** (no recipe): node, openjdk, python@3.14, yuicompressor

### enter-tex

- **Formula**: `enter-tex`
- **Current**: `["gettext", "glib"]`
- **Full correct**: `[adwaita-icon-theme (*), gdk-pixbuf (*), gettext, glib, gspell (*), gtk+3 (*), libgedit-amtk (*), libgedit-gtksourceview (*), libgedit-tepl (*), libgee (*), pango (*)]`
- **Blocked** (no recipe): adwaita-icon-theme, gdk-pixbuf, gspell, gtk+3, libgedit-amtk, libgedit-gtksourceview, libgedit-tepl, libgee, pango

### epstool

- **Formula**: `epstool`
- **Current**: `[]`
- **Full correct**: `[ghostscript (*)]`
- **Blocked** (no recipe): ghostscript

### erlang-language-platform

- **Formula**: `erlang-language-platform`
- **Current**: `[]`
- **Full correct**: `[erlang (*), openjdk (*), rebar3]`
- **Can add now**: rebar3
- **Blocked** (no recipe): erlang, openjdk

### ettercap

- **Formula**: `ettercap`
- **Current**: `["cairo", "freetype", "glib", "ncurses", "pcre2"]`
- **Full correct**: `[at-spi2-core (*), cairo, freetype, gdk-pixbuf (*), glib, gtk+3 (*), libmaxminddb (*), libnet (*), ncurses, openssl@3 (*), pango (*), pcre2]`
- **Blocked** (no recipe): at-spi2-core, gdk-pixbuf, gtk+3, libmaxminddb, libnet, openssl@3, pango

### fail2ban

- **Formula**: `fail2ban`
- **Current**: `[]`
- **Full correct**: `[python@3.14 (*)]`
- **Blocked** (no recipe): python@3.14

### faircamp

- **Formula**: `faircamp`
- **Current**: `["gettext", "glib", "xz"]`
- **Full correct**: `[ffmpeg, gettext, glib, opus (*), vips (*), xz]`
- **Can add now**: ffmpeg
- **Blocked** (no recipe): opus, vips

### fancy-cat

- **Formula**: `fancy-cat`
- **Current**: `[]`
- **Full correct**: `[mujs (*), mupdf (*)]`
- **Blocked** (no recipe): mujs, mupdf

### fastfetch

- **Formula**: `fastfetch`
- **Current**: `[]`
- **Full correct**: `[yyjson (*)]`
- **Blocked** (no recipe): yyjson

### fastnetmon

- **Formula**: `fastnetmon`
- **Current**: `["abseil", "capnp"]`
- **Full correct**: `[abseil, boost (*), capnp, grpc (*), hiredis (*), log4cpp (*), mongo-c-driver, openssl@3 (*), protobuf (*)]`
- **Can add now**: mongo-c-driver
- **Blocked** (no recipe): boost, grpc, hiredis, log4cpp, openssl@3, protobuf

### fastp

- **Formula**: `fastp`
- **Current**: `[]`
- **Full correct**: `[isa-l (*), libdeflate (*)]`
- **Blocked** (no recipe): isa-l, libdeflate

### fatsort

- **Formula**: `fatsort`
- **Current**: `[]`
- **Full correct**: `[help2man (*)]`
- **Blocked** (no recipe): help2man

### fceux

- **Formula**: `fceux`
- **Current**: `["sdl2"]`
- **Full correct**: `[ffmpeg, libarchive (*), minizip (*), qtbase, sdl2, x264 (*), x265]`
- **Can add now**: ffmpeg, qtbase, x265
- **Blocked** (no recipe): libarchive, minizip, x264

### feedgnuplot

- **Formula**: `feedgnuplot`
- **Current**: `[]`
- **Full correct**: `[gnuplot (*)]`
- **Blocked** (no recipe): gnuplot

### feh

- **Formula**: `feh`
- **Current**: `["libpng"]`
- **Full correct**: `[imlib2, libexif (*), libpng, libx11 (*), libxinerama (*), libxt (*)]`
- **Can add now**: imlib2
- **Blocked** (no recipe): libexif, libx11, libxinerama, libxt

### felinks

- **Formula**: `felinks`
- **Current**: `["brotli", "libidn2", "zstd"]`
- **Full correct**: `[brotli, libcss (*), libdom (*), libidn2, libwapcaplet (*), openssl@3 (*), tre (*), zstd]`
- **Blocked** (no recipe): libcss, libdom, libwapcaplet, openssl@3, tre

### fetchmail

- **Formula**: `fetchmail`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### ffmpeg

- **Formula**: `ffmpeg`
- **Current**: `["dav1d", "sdl2", "svt-av1", "x265"]`
- **Full correct**: `[dav1d, lame (*), libvpx (*), openssl@3 (*), opus (*), sdl2, svt-av1, x264 (*), x265]`
- **Blocked** (no recipe): lame, libvpx, openssl@3, opus, x264

### ffmpeg-full

- **Formula**: `ffmpeg-full`
- **Current**: `["dav1d", "fontconfig", "freetype", "gnutls", "rav1e", "sdl2", "snappy", "xz"]`
- **Full correct**: `[aom (*), aribb24 (*), dav1d, fontconfig, freetype, frei0r (*), gnutls, harfbuzz (*), jpeg-xl, lame (*), libarchive (*), libass (*), libbluray, libogg (*), libplacebo (*), librist, libsamplerate (*), libsoxr (*), libssh (*), libvidstab (*), libvmaf, libvorbis (*), libvpx (*), libx11 (*), libxcb (*), llama.cpp (*), opencore-amr (*), openjpeg (*), opus (*), rav1e, rubberband (*), sdl2, snappy, speex, srt (*), svt-av1, tesseract (*), theora (*), webp (*), whisper-cpp (*), x264 (*), x265, xvid (*), xz, zeromq (*), zimg (*)]`
- **Can add now**: jpeg-xl, libbluray, librist, libvmaf, speex, svt-av1, x265
- **Blocked** (no recipe): aom, aribb24, frei0r, harfbuzz, lame, libarchive, libass, libogg, libplacebo, libsamplerate, libsoxr, libssh, libvidstab, libvorbis, libvpx, libx11, libxcb, llama.cpp, opencore-amr, openjpeg, opus, rubberband, srt, tesseract, theora, webp, whisper-cpp, x264, xvid, zeromq, zimg

### ffmpegthumbnailer

- **Formula**: `ffmpegthumbnailer`
- **Current**: `["jpeg-turbo", "libpng"]`
- **Full correct**: `[ffmpeg, jpeg-turbo, libpng]`
- **Can add now**: ffmpeg

### fheroes2

- **Formula**: `fheroes2`
- **Current**: `["sdl2"]`
- **Full correct**: `[innoextract, sdl2, sdl2_mixer (*)]`
- **Can add now**: innoextract
- **Blocked** (no recipe): sdl2_mixer

### fig2dev

- **Formula**: `fig2dev`
- **Current**: `["libpng"]`
- **Full correct**: `[ghostscript (*), libpng, netpbm (*)]`
- **Blocked** (no recipe): ghostscript, netpbm

### file-formula

- **Formula**: `file-formula`
- **Current**: `[]`
- **Full correct**: `[libmagic (*)]`
- **Blocked** (no recipe): libmagic

### fluent-bit

- **Formula**: `fluent-bit`
- **Current**: `["libyaml", "luajit"]`
- **Full correct**: `[libyaml, luajit, openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### fluid-synth

- **Formula**: `fluid-synth`
- **Current**: `["gettext", "glib", "readline"]`
- **Full correct**: `[gettext, glib, libsndfile, portaudio (*), readline]`
- **Can add now**: libsndfile
- **Blocked** (no recipe): portaudio

### fontconfig

- **Formula**: `fontconfig`
- **Current**: `[]`
- **Full correct**: `[freetype, gettext]`
- **Can add now**: freetype, gettext

### fontforge

- **Formula**: `fontforge`
- **Current**: `["brotli", "cairo", "fontconfig", "freetype", "gettext", "giflib", "glib", "jpeg-turbo", "libpng", "readline"]`
- **Full correct**: `[brotli, cairo, fontconfig, freetype, gettext, giflib, glib, jpeg-turbo, libpng, libspiro (*), libtiff (*), libtool (*), libuninameslist (*), pango (*), python@3.14 (*), readline, woff2 (*)]`
- **Blocked** (no recipe): libspiro, libtiff, libtool, libuninameslist, pango, python@3.14, woff2

### fq

- **Formula**: `fq`
- **Current**: `[]`
- **Full correct**: `[concurrencykit (*), jlog (*)]`
- **Blocked** (no recipe): concurrencykit, jlog

### freeciv

- **Formula**: `freeciv`
- **Current**: `["cairo", "freetype", "gettext", "glib", "readline", "sdl2", "sqlite", "zstd"]`
- **Full correct**: `[adwaita-icon-theme (*), at-spi2-core (*), cairo, freetype, gdk-pixbuf (*), gettext, glib, gtk+3 (*), harfbuzz (*), icu4c@78 (*), pango (*), readline, sdl2, sdl2_mixer (*), sqlite, zstd]`
- **Blocked** (no recipe): adwaita-icon-theme, at-spi2-core, gdk-pixbuf, gtk+3, harfbuzz, icu4c@78, pango, sdl2_mixer

### freeradius-server

- **Formula**: `freeradius-server`
- **Current**: `["readline", "sqlite"]`
- **Full correct**: `[collectd (*), json-c (*), openssl@3 (*), python@3.14 (*), readline, sqlite, talloc (*)]`
- **Blocked** (no recipe): collectd, json-c, openssl@3, python@3.14, talloc

### freetds

- **Formula**: `freetds`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*), unixodbc]`
- **Can add now**: unixodbc
- **Blocked** (no recipe): openssl@3

### fricas

- **Formula**: `fricas`
- **Current**: `["gmp", "zstd"]`
- **Full correct**: `[gmp, libice (*), libsm (*), libx11 (*), libxau (*), libxdmcp (*), libxpm, libxt (*), sbcl, zstd]`
- **Can add now**: libxpm, sbcl
- **Blocked** (no recipe): libice, libsm, libx11, libxau, libxdmcp, libxt

### frotz

- **Formula**: `frotz`
- **Current**: `["freetype", "jpeg-turbo", "libpng", "ncurses", "sdl2"]`
- **Full correct**: `[freetype, jpeg-turbo, libao (*), libmodplug (*), libpng, libsamplerate (*), libsndfile, libvorbis (*), ncurses, sdl2, sdl2_mixer (*)]`
- **Can add now**: libsndfile
- **Blocked** (no recipe): libao, libmodplug, libsamplerate, libvorbis, sdl2_mixer

### fwup

- **Formula**: `fwup`
- **Current**: `[]`
- **Full correct**: `[confuse (*), libarchive (*)]`
- **Blocked** (no recipe): confuse, libarchive

### fwupd

- **Formula**: `fwupd`
- **Current**: `["gettext", "glib", "gnutls", "readline", "sqlite", "xz"]`
- **Full correct**: `[gettext, glib, gnutls, json-glib, libarchive (*), libcbor (*), libjcat (*), libusb (*), libxmlb, protobuf-c, readline, sqlite, usb.ids (*), xz]`
- **Can add now**: json-glib, libxmlb, protobuf-c
- **Blocked** (no recipe): libarchive, libcbor, libjcat, libusb, usb.ids

### gabedit

- **Formula**: `gabedit`
- **Current**: `["cairo", "gettext", "glib"]`
- **Full correct**: `[at-spi2-core (*), cairo, gdk-pixbuf (*), gettext, glib, gtk+ (*), gtkglext (*), harfbuzz (*), pango (*)]`
- **Blocked** (no recipe): at-spi2-core, gdk-pixbuf, gtk+, gtkglext, harfbuzz, pango

### gambit-scheme

- **Formula**: `gambit-scheme`
- **Current**: `[]`
- **Full correct**: `[gcc (*), openssl@3 (*)]`
- **Blocked** (no recipe): gcc, openssl@3

### gammaray

- **Formula**: `gammaray`
- **Current**: `[]`
- **Full correct**: `[graphviz (*), qt3d (*), qtbase, qtconnectivity (*), qtdeclarative, qtlocation (*), qtpositioning (*), qtscxml (*), qtsvg (*), qttools, qtwebchannel (*), qtwebengine (*)]`
- **Can add now**: qtbase, qtdeclarative, qttools
- **Blocked** (no recipe): graphviz, qt3d, qtconnectivity, qtlocation, qtpositioning, qtscxml, qtsvg, qtwebchannel, qtwebengine

### gastown

- **Formula**: `gastown`
- **Current**: `[]`
- **Full correct**: `[beads (*), icu4c@78 (*)]`
- **Blocked** (no recipe): beads, icu4c@78

### gauche

- **Formula**: `gauche`
- **Current**: `["ca-certificates"]`
- **Full correct**: `[ca-certificates, mbedtls@3 (*)]`
- **Blocked** (no recipe): mbedtls@3

### gedit

- **Formula**: `gedit`
- **Current**: `["cairo", "gettext", "glib"]`
- **Full correct**: `[adwaita-icon-theme (*), cairo, gdk-pixbuf (*), gettext, glib, gobject-introspection (*), gsettings-desktop-schemas (*), gspell (*), gtk+3 (*), gtk-mac-integration (*), libgedit-amtk (*), libgedit-gfls (*), libgedit-gtksourceview (*), libgedit-tepl (*), libpeas@1 (*), pango (*)]`
- **Blocked** (no recipe): adwaita-icon-theme, gdk-pixbuf, gobject-introspection, gsettings-desktop-schemas, gspell, gtk+3, gtk-mac-integration, libgedit-amtk, libgedit-gfls, libgedit-gtksourceview, libgedit-tepl, libpeas@1, pango

### geeqie

- **Formula**: `geeqie`
- **Current**: `["cairo", "djvulibre", "exiv2", "ffmpegthumbnailer", "gettext", "glib", "jpeg-turbo"]`
- **Full correct**: `[adwaita-icon-theme (*), cairo, djvulibre, exiv2, ffmpegthumbnailer, gdk-pixbuf (*), gettext, glib, gspell (*), gtk+3 (*), imagemagick (*), imath (*), jpeg-turbo, jpeg-xl, libarchive (*), libheif, libraw (*), libtiff (*), little-cms2, openexr (*), openjpeg (*), pango (*), poppler (*), webp (*)]`
- **Can add now**: jpeg-xl, libheif, little-cms2
- **Blocked** (no recipe): adwaita-icon-theme, gdk-pixbuf, gspell, gtk+3, imagemagick, imath, libarchive, libraw, libtiff, openexr, openjpeg, pango, poppler, webp

### gensio

- **Formula**: `gensio`
- **Current**: `["gettext", "glib"]`
- **Full correct**: `[gettext, glib, openssl@3 (*), portaudio (*), python@3.14 (*), tcl-tk]`
- **Can add now**: tcl-tk
- **Blocked** (no recipe): openssl@3, portaudio, python@3.14

### gerbv

- **Formula**: `gerbv`
- **Current**: `["cairo", "gettext", "glib"]`
- **Full correct**: `[at-spi2-core (*), cairo, gdk-pixbuf (*), gettext, glib, gtk+ (*), harfbuzz (*), pango (*)]`
- **Blocked** (no recipe): at-spi2-core, gdk-pixbuf, gtk+, harfbuzz, pango

### get-iplayer

- **Formula**: `get_iplayer`
- **Current**: `["atomicparsley"]`
- **Full correct**: `[atomicparsley, ffmpeg]`
- **Can add now**: ffmpeg

### gettext

- **Formula**: `gettext`
- **Current**: `[]`
- **Full correct**: `[libunistring (*)]`
- **Blocked** (no recipe): libunistring

### ghidra

- **Formula**: `ghidra`
- **Current**: `[]`
- **Full correct**: `[openjdk@21 (*)]`
- **Blocked** (no recipe): openjdk@21

### ginac

- **Formula**: `ginac`
- **Current**: `["readline"]`
- **Full correct**: `[cln (*), python@3.14 (*), readline]`
- **Blocked** (no recipe): cln, python@3.14

### git

- **Formula**: `git`
- **Current**: `[]`
- **Full correct**: `[gettext, libiconv (*), pcre2]`
- **Can add now**: gettext, pcre2
- **Blocked** (no recipe): libiconv

### git-credential-libsecret

- **Formula**: `git-credential-libsecret`
- **Current**: `["gettext", "glib"]`
- **Full correct**: `[gettext, glib, libsecret (*)]`
- **Blocked** (no recipe): libsecret

### git-crypt

- **Formula**: `git-crypt`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### git-xet

- **Formula**: `git-xet`
- **Current**: `["git-lfs"]`
- **Full correct**: `[git-lfs, openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### gitlogue

- **Formula**: `gitlogue`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### gitmux

- **Formula**: `gitmux`
- **Current**: `[]`
- **Full correct**: `[tmux (*)]`
- **Blocked** (no recipe): tmux

### gkrellm

- **Formula**: `gkrellm`
- **Current**: `["gettext", "glib"]`
- **Full correct**: `[gdk-pixbuf (*), gettext, glib, gtk+ (*), openssl@3 (*), pango (*)]`
- **Blocked** (no recipe): gdk-pixbuf, gtk+, openssl@3, pango

### glib

- **Formula**: `glib`
- **Current**: `[]`
- **Full correct**: `[gettext, pcre2]`
- **Can add now**: gettext, pcre2

### glslviewer

- **Formula**: `glslviewer`
- **Current**: `[]`
- **Full correct**: `[ffmpeg, glfw (*)]`
- **Can add now**: ffmpeg
- **Blocked** (no recipe): glfw

### gnome-builder

- **Formula**: `gnome-builder`
- **Current**: `["cairo", "cmark", "gettext", "glib", "libgit2", "libyaml"]`
- **Full correct**: `[adwaita-icon-theme (*), cairo, cmark, editorconfig (*), gettext, glib, gobject-introspection (*), gtk4 (*), gtksourceview5 (*), json-glib, jsonrpc-glib (*), libadwaita (*), libdex (*), libgit2, libgit2-glib (*), libpanel (*), libpeas (*), libspelling (*), libyaml, llvm (*), pango (*), template-glib (*), vte3 (*)]`
- **Can add now**: json-glib
- **Blocked** (no recipe): adwaita-icon-theme, editorconfig, gobject-introspection, gtk4, gtksourceview5, jsonrpc-glib, libadwaita, libdex, libgit2-glib, libpanel, libpeas, libspelling, llvm, pango, template-glib, vte3

### gnome-papers

- **Formula**: `gnome-papers`
- **Current**: `["cairo", "djvulibre", "gettext", "glib"]`
- **Full correct**: `[adwaita-icon-theme (*), cairo, djvulibre, exempi (*), gdk-pixbuf (*), gettext, glib, graphene (*), gtk4 (*), gtksourceview5 (*), harfbuzz (*), hicolor-icon-theme (*), libadwaita (*), libarchive (*), libspelling (*), libtiff (*), pango (*), poppler (*)]`
- **Blocked** (no recipe): adwaita-icon-theme, exempi, gdk-pixbuf, graphene, gtk4, gtksourceview5, harfbuzz, hicolor-icon-theme, libadwaita, libarchive, libspelling, libtiff, pango, poppler

### gnu-apl

- **Formula**: `gnu-apl`
- **Current**: `["cairo", "gettext", "glib", "libpng", "pcre2", "readline", "sqlite"]`
- **Full correct**: `[at-spi2-core (*), cairo, gdk-pixbuf (*), gettext, glib, gtk+3 (*), harfbuzz (*), libpng, libx11 (*), libxcb (*), pango (*), pcre2, readline, sqlite]`
- **Blocked** (no recipe): at-spi2-core, gdk-pixbuf, gtk+3, harfbuzz, libx11, libxcb, pango

### gnuastro

- **Formula**: `gnuastro`
- **Current**: `["cfitsio", "gsl", "jpeg-turbo", "libgit2"]`
- **Full correct**: `[cfitsio, gsl, jpeg-turbo, libgit2, libtiff (*), libtool (*), wcslib (*)]`
- **Blocked** (no recipe): libtiff, libtool, wcslib

### gnucobol

- **Formula**: `gnucobol`
- **Current**: `["berkeley-db", "gmp"]`
- **Full correct**: `[berkeley-db, gmp, json-c (*)]`
- **Blocked** (no recipe): json-c

### gnuradio

- **Formula**: `gnuradio`
- **Current**: `["gmp", "gsl", "jack", "libyaml", "numpy"]`
- **Full correct**: `[adwaita-icon-theme (*), boost (*), cppzmq (*), fftw (*), fmt (*), gmp, gsl, gtk+3 (*), jack, libsndfile, libyaml, numpy, portaudio (*), pygobject3 (*), pyqt@5 (*), python@3.14 (*), qt@5 (*), qwt-qt5 (*), rpds-py (*), soapyrtlsdr (*), soapysdr (*), spdlog (*), uhd (*), volk (*), zeromq (*)]`
- **Can add now**: libsndfile
- **Blocked** (no recipe): adwaita-icon-theme, boost, cppzmq, fftw, fmt, gtk+3, portaudio, pygobject3, pyqt@5, python@3.14, qt@5, qwt-qt5, rpds-py, soapyrtlsdr, soapysdr, spdlog, uhd, volk, zeromq

### gnutls

- **Formula**: `gnutls`
- **Current**: `["ca-certificates", "gettext", "gmp", "libidn2"]`
- **Full correct**: `[ca-certificates, gettext, gmp, libidn2, libtasn1, libunistring (*), nettle (*), p11-kit, unbound (*)]`
- **Can add now**: libtasn1, p11-kit
- **Blocked** (no recipe): libunistring, nettle, unbound

### goaccess

- **Formula**: `goaccess`
- **Current**: `["gettext"]`
- **Full correct**: `[gettext, libmaxminddb (*)]`
- **Blocked** (no recipe): libmaxminddb

### gopass

- **Formula**: `gopass`
- **Current**: `[]`
- **Full correct**: `[gnupg (*), terminal-notifier (*)]`
- **Blocked** (no recipe): gnupg, terminal-notifier

### gource

- **Formula**: `gource`
- **Current**: `["freetype", "glew", "libpng", "pcre2", "sdl2"]`
- **Full correct**: `[boost (*), freetype, glew, libpng, pcre2, sdl2, sdl2_image (*)]`
- **Blocked** (no recipe): boost, sdl2_image

### gowall

- **Formula**: `gowall`
- **Current**: `[]`
- **Full correct**: `[mupdf (*)]`
- **Blocked** (no recipe): mupdf

### gplugin

- **Formula**: `gplugin`
- **Current**: `["gettext", "glib"]`
- **Full correct**: `[gettext, glib, gtk4 (*), pygobject3 (*), python@3.14 (*)]`
- **Blocked** (no recipe): gtk4, pygobject3, python@3.14

### gpredict

- **Formula**: `gpredict`
- **Current**: `["cairo", "gettext", "glib"]`
- **Full correct**: `[adwaita-icon-theme (*), at-spi2-core (*), cairo, gdk-pixbuf (*), gettext, glib, goocanvas (*), gtk+3 (*), hamlib (*), harfbuzz (*), pango (*)]`
- **Blocked** (no recipe): adwaita-icon-theme, at-spi2-core, gdk-pixbuf, goocanvas, gtk+3, hamlib, harfbuzz, pango

### gptfdisk

- **Formula**: `gptfdisk`
- **Current**: `[]`
- **Full correct**: `[popt (*)]`
- **Blocked** (no recipe): popt

### groff

- **Formula**: `groff`
- **Current**: `[]`
- **Full correct**: `[ghostscript (*), netpbm (*), psutils (*), uchardet (*)]`
- **Blocked** (no recipe): ghostscript, netpbm, psutils, uchardet

### grokj2k

- **Formula**: `grokj2k`
- **Current**: `["exiftool", "jpeg-turbo", "libpng", "little-cms2", "xz", "zstd"]`
- **Full correct**: `[exiftool, jpeg-turbo, libpng, libtiff (*), little-cms2, xz, zstd]`
- **Blocked** (no recipe): libtiff

### gromacs

- **Formula**: `gromacs`
- **Current**: `[]`
- **Full correct**: `[fftw (*), libomp (*), lmfit (*), muparser (*), openblas (*)]`
- **Blocked** (no recipe): fftw, libomp, lmfit, muparser, openblas

### gsmartcontrol

- **Formula**: `gsmartcontrol`
- **Current**: `["cairo", "gettext", "glib", "pcre2", "smartmontools"]`
- **Full correct**: `[at-spi2-core (*), atkmm@2.28 (*), cairo, cairomm@1.14 (*), gdk-pixbuf (*), gettext, glib, glibmm@2.66 (*), gtk+3 (*), gtkmm3 (*), harfbuzz (*), libsigc++@2 (*), pango (*), pangomm@2.46 (*), pcre2, smartmontools]`
- **Blocked** (no recipe): at-spi2-core, atkmm@2.28, cairomm@1.14, gdk-pixbuf, glibmm@2.66, gtk+3, gtkmm3, harfbuzz, libsigc++@2, pango, pangomm@2.46

### gsoap

- **Formula**: `gsoap`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### gtk-doc

- **Formula**: `gtk-doc`
- **Current**: `[]`
- **Full correct**: `[docbook (*), docbook-xsl (*), python@3.14 (*)]`
- **Blocked** (no recipe): docbook, docbook-xsl, python@3.14

### gtk-gnutella

- **Formula**: `gtk-gnutella`
- **Current**: `["gettext", "glib"]`
- **Full correct**: `[at-spi2-core (*), dbus (*), gdk-pixbuf (*), gettext, glib, gtk+ (*), harfbuzz (*), pango (*)]`
- **Blocked** (no recipe): at-spi2-core, dbus, gdk-pixbuf, gtk+, harfbuzz, pango

### gtk-vnc

- **Formula**: `gtk-vnc`
- **Current**: `["cairo", "gettext", "glib", "gmp", "gnutls", "libgcrypt"]`
- **Full correct**: `[cairo, gdk-pixbuf (*), gettext, glib, gmp, gnutls, gtk+3 (*), libgcrypt]`
- **Blocked** (no recipe): gdk-pixbuf, gtk+3

### gtranslator

- **Formula**: `gtranslator`
- **Current**: `["cairo", "gettext", "glib", "json-glib", "sqlite"]`
- **Full correct**: `[adwaita-icon-theme (*), cairo, gettext, glib, gtk4 (*), gtksourceview5 (*), json-glib, libadwaita (*), libsoup (*), libspelling (*), pango (*), sqlite]`
- **Blocked** (no recipe): adwaita-icon-theme, gtk4, gtksourceview5, libadwaita, libsoup, libspelling, pango

### gucharmap

- **Formula**: `gucharmap`
- **Current**: `["cairo", "gettext", "glib", "pcre2"]`
- **Full correct**: `[at-spi2-core (*), cairo, gettext, glib, gtk+3 (*), pango (*), pcre2]`
- **Blocked** (no recipe): at-spi2-core, gtk+3, pango

### guile

- **Formula**: `guile`
- **Current**: `["bdw-gc", "gmp", "pkgconf", "readline"]`
- **Full correct**: `[bdw-gc, gmp, libtool (*), libunistring (*), pkgconf, readline]`
- **Blocked** (no recipe): libtool, libunistring

### gupnp

- **Formula**: `gupnp`
- **Current**: `["glib", "libxml2"]`
- **Full correct**: `[glib, gssdp (*), libsoup (*), libxml2, python@3.14 (*)]`
- **Blocked** (no recipe): gssdp, libsoup, python@3.14

### gupnp-tools

- **Formula**: `gupnp-tools`
- **Current**: `["gettext", "glib", "gupnp", "libxml2"]`
- **Full correct**: `[gdk-pixbuf (*), gettext, glib, gssdp (*), gtk+3 (*), gtksourceview4 (*), gupnp, gupnp-av (*), libsoup (*), libxml2]`
- **Blocked** (no recipe): gdk-pixbuf, gssdp, gtk+3, gtksourceview4, gupnp-av, libsoup

### gwenhywfar

- **Formula**: `gwenhywfar`
- **Current**: `["gettext", "gnutls", "libgcrypt", "libgpg-error", "pkgconf"]`
- **Full correct**: `[gettext, gnutls, libgcrypt, libgpg-error, openssl@3 (*), pkgconf, qtbase]`
- **Can add now**: qtbase
- **Blocked** (no recipe): openssl@3

### gwyddion

- **Formula**: `gwyddion`
- **Current**: `["cairo", "gettext", "glib", "libpng", "libxml2"]`
- **Full correct**: `[at-spi2-core (*), cairo, fftw (*), gdk-pixbuf (*), gettext, glib, gtk+ (*), gtkglext (*), harfbuzz (*), libpng, libxml2, minizip (*), pango (*)]`
- **Blocked** (no recipe): at-spi2-core, fftw, gdk-pixbuf, gtk+, gtkglext, harfbuzz, minizip, pango

### gyb

- **Formula**: `gyb`
- **Current**: `[]`
- **Full correct**: `[certifi (*), python@3.14 (*)]`
- **Blocked** (no recipe): certifi, python@3.14

### hcxtools

- **Formula**: `hcxtools`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### hdf5-mpi

- **Formula**: `hdf5-mpi`
- **Current**: `["open-mpi", "pkgconf"]`
- **Full correct**: `[gcc (*), libaec (*), open-mpi, pkgconf]`
- **Blocked** (no recipe): gcc, libaec

### homebank

- **Formula**: `homebank`
- **Current**: `["cairo", "fontconfig", "freetype", "gettext", "glib"]`
- **Full correct**: `[adwaita-icon-theme (*), at-spi2-core (*), cairo, fontconfig, freetype, gdk-pixbuf (*), gettext, glib, gtk+3 (*), harfbuzz (*), hicolor-icon-theme (*), libofx (*), libsoup (*), pango (*)]`
- **Blocked** (no recipe): adwaita-icon-theme, at-spi2-core, gdk-pixbuf, gtk+3, harfbuzz, hicolor-icon-theme, libofx, libsoup, pango

### hopenpgp-tools

- **Formula**: `hopenpgp-tools`
- **Current**: `["gmp"]`
- **Full correct**: `[gmp, nettle (*)]`
- **Blocked** (no recipe): nettle

### httpd

- **Formula**: `httpd`
- **Current**: `["apr", "apr-util", "brotli", "libnghttp2", "pcre2"]`
- **Full correct**: `[apr, apr-util, brotli, libnghttp2, openssl@3 (*), pcre2]`
- **Blocked** (no recipe): openssl@3

### i2pd

- **Formula**: `i2pd`
- **Current**: `[]`
- **Full correct**: `[boost (*), miniupnpc (*), openssl@3 (*)]`
- **Blocked** (no recipe): boost, miniupnpc, openssl@3

### i386-elf-gdb

- **Formula**: `i386-elf-gdb`
- **Current**: `["gmp", "ncurses", "readline", "xz", "zstd"]`
- **Full correct**: `[gmp, mpfr (*), ncurses, python@3.14 (*), readline, xz, zstd]`
- **Blocked** (no recipe): mpfr, python@3.14

### i686-elf-gcc

- **Formula**: `i686-elf-gcc`
- **Current**: `["gmp", "i686-elf-binutils", "zstd"]`
- **Full correct**: `[gmp, i686-elf-binutils, libmpc (*), mpfr (*), zstd]`
- **Blocked** (no recipe): libmpc, mpfr

### icann-rdap

- **Formula**: `icann-rdap`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### icp-cli

- **Formula**: `icp-cli`
- **Current**: `[]`
- **Full correct**: `[ic-wasm (*), openssl@3 (*)]`
- **Blocked** (no recipe): ic-wasm, openssl@3

### ideviceinstaller

- **Formula**: `ideviceinstaller`
- **Current**: `["libplist"]`
- **Full correct**: `[libimobiledevice, libplist, libzip (*)]`
- **Can add now**: libimobiledevice
- **Blocked** (no recipe): libzip

### ike-scan

- **Formula**: `ike-scan`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### imagemagick-full

- **Formula**: `imagemagick-full`
- **Current**: `["cairo", "fontconfig", "freetype", "gettext", "glib", "jpeg-turbo", "jpeg-xl", "libheif", "libpng", "little-cms2", "xz"]`
- **Full correct**: `[cairo, fontconfig, freetype, gdk-pixbuf (*), gettext, ghostscript (*), glib, imath (*), jpeg-turbo, jpeg-xl, libheif, liblqr (*), libomp (*), libpng, libraw (*), librsvg (*), libtiff (*), libtool (*), libultrahdr (*), libzip (*), little-cms2, openexr (*), openjpeg (*), webp (*), xz]`
- **Blocked** (no recipe): gdk-pixbuf, ghostscript, imath, liblqr, libomp, libraw, librsvg, libtiff, libtool, libultrahdr, libzip, openexr, openjpeg, webp

### imlib2

- **Formula**: `imlib2`
- **Current**: `["freetype", "giflib", "jpeg-turbo", "libpng", "xz"]`
- **Full correct**: `[freetype, giflib, jpeg-turbo, libpng, libtiff (*), libx11 (*), libxcb (*), libxext (*), xz]`
- **Blocked** (no recipe): libtiff, libx11, libxcb, libxext

### include-what-you-use

- **Formula**: `include-what-you-use`
- **Current**: `[]`
- **Full correct**: `[llvm@21 (*)]`
- **Blocked** (no recipe): llvm@21

### innoextract

- **Formula**: `innoextract`
- **Current**: `["xz"]`
- **Full correct**: `[boost (*), xz]`
- **Blocked** (no recipe): boost

### inspectrum

- **Formula**: `inspectrum`
- **Current**: `[]`
- **Full correct**: `[fftw (*), liquid-dsp (*), qtbase]`
- **Can add now**: qtbase
- **Blocked** (no recipe): fftw, liquid-dsp

### ios-webkit-debug-proxy

- **Formula**: `ios-webkit-debug-proxy`
- **Current**: `["libplist", "libusbmuxd"]`
- **Full correct**: `[libimobiledevice, libplist, libusbmuxd, openssl@3 (*)]`
- **Can add now**: libimobiledevice
- **Blocked** (no recipe): openssl@3

### ipmitool

- **Formula**: `ipmitool`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### ircii

- **Formula**: `ircii`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### irssi

- **Formula**: `irssi`
- **Current**: `["gettext", "glib", "perl"]`
- **Full correct**: `[gettext, glib, openssl@3 (*), perl]`
- **Blocked** (no recipe): openssl@3

### jackett

- **Formula**: `jackett`
- **Current**: `[]`
- **Full correct**: `[dotnet@9 (*)]`
- **Blocked** (no recipe): dotnet@9

### jags

- **Formula**: `jags`
- **Current**: `[]`
- **Full correct**: `[openblas (*)]`
- **Blocked** (no recipe): openblas

### javacc

- **Formula**: `javacc`
- **Current**: `[]`
- **Full correct**: `[openjdk (*)]`
- **Blocked** (no recipe): openjdk

### jbig2enc

- **Formula**: `jbig2enc`
- **Current**: `["giflib", "jpeg-turbo", "leptonica", "libpng"]`
- **Full correct**: `[giflib, jpeg-turbo, leptonica, libpng, libtiff (*), webp (*)]`
- **Blocked** (no recipe): libtiff, webp

### jdupes

- **Formula**: `jdupes`
- **Current**: `[]`
- **Full correct**: `[libjodycode (*)]`
- **Blocked** (no recipe): libjodycode

### jhipster

- **Formula**: `jhipster`
- **Current**: `[]`
- **Full correct**: `[node (*), openjdk (*)]`
- **Blocked** (no recipe): node, openjdk

### jimtcl

- **Formula**: `jimtcl`
- **Current**: `["readline"]`
- **Full correct**: `[openssl@3 (*), readline]`
- **Blocked** (no recipe): openssl@3

### joern

- **Formula**: `joern`
- **Current**: `[]`
- **Full correct**: `[astgen (*), coreutils (*), openjdk (*), php]`
- **Can add now**: php
- **Blocked** (no recipe): astgen, coreutils, openjdk

### jp2a

- **Formula**: `jp2a`
- **Current**: `["jpeg-turbo", "libpng"]`
- **Full correct**: `[jpeg-turbo, libexif (*), libpng, webp (*)]`
- **Blocked** (no recipe): libexif, webp

### jpeg-xl

- **Formula**: `jpeg-xl`
- **Current**: `["brotli", "giflib", "jpeg-turbo", "libpng", "little-cms2"]`
- **Full correct**: `[brotli, giflib, highway (*), imath (*), jpeg-turbo, libpng, little-cms2, openexr (*)]`
- **Blocked** (no recipe): highway, imath, openexr

### jruby

- **Formula**: `jruby`
- **Current**: `[]`
- **Full correct**: `[libfixposix (*), openjdk (*)]`
- **Blocked** (no recipe): libfixposix, openjdk

### jsonschema2pojo

- **Formula**: `jsonschema2pojo`
- **Current**: `[]`
- **Full correct**: `[openjdk (*)]`
- **Blocked** (no recipe): openjdk

### jython

- **Formula**: `jython`
- **Current**: `[]`
- **Full correct**: `[openjdk (*)]`
- **Blocked** (no recipe): openjdk

### katago

- **Formula**: `katago`
- **Current**: `[]`
- **Full correct**: `[libzip (*)]`
- **Blocked** (no recipe): libzip

### kcov

- **Formula**: `kcov`
- **Current**: `["dwarfutils"]`
- **Full correct**: `[dwarfutils, openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### kdoctools

- **Formula**: `kdoctools`
- **Current**: `[]`
- **Full correct**: `[docbook-xsl (*), karchive (*), qtbase]`
- **Can add now**: qtbase
- **Blocked** (no recipe): docbook-xsl, karchive

### kiota

- **Formula**: `kiota`
- **Current**: `[]`
- **Full correct**: `[dotnet (*)]`
- **Blocked** (no recipe): dotnet

### klavaro

- **Formula**: `klavaro`
- **Current**: `["cairo", "gettext", "glib"]`
- **Full correct**: `[adwaita-icon-theme (*), at-spi2-core (*), cairo, gdk-pixbuf (*), gettext, glib, gtk+3 (*), gtkdatabox (*), harfbuzz (*), pango (*)]`
- **Blocked** (no recipe): adwaita-icon-theme, at-spi2-core, gdk-pixbuf, gtk+3, gtkdatabox, harfbuzz, pango

### knot-resolver

- **Formula**: `knot-resolver`
- **Current**: `["gnutls", "libnghttp2", "libyaml", "luajit"]`
- **Full correct**: `[fstrm (*), gnutls, knot (*), libnghttp2, libuv (*), libyaml, lmdb (*), luajit, protobuf-c, python@3.14 (*)]`
- **Can add now**: protobuf-c
- **Blocked** (no recipe): fstrm, knot, libuv, lmdb, python@3.14

### kotlin-language-server

- **Formula**: `kotlin-language-server`
- **Current**: `[]`
- **Full correct**: `[openjdk@21 (*)]`
- **Blocked** (no recipe): openjdk@21

### kubekey

- **Formula**: `kubekey`
- **Current**: `[]`
- **Full correct**: `[gpgme (*)]`
- **Blocked** (no recipe): gpgme

### lanraragi

- **Formula**: `lanraragi`
- **Current**: `["perl", "redis", "zstd"]`
- **Full correct**: `[cpanminus (*), ghostscript (*), imagemagick (*), libarchive (*), node (*), openssl@3 (*), perl, redis, zstd]`
- **Blocked** (no recipe): cpanminus, ghostscript, imagemagick, libarchive, node, openssl@3

### latex2html

- **Formula**: `latex2html`
- **Current**: `[]`
- **Full correct**: `[ghostscript (*), netpbm (*)]`
- **Blocked** (no recipe): ghostscript, netpbm

### lbdb

- **Formula**: `lbdb`
- **Current**: `[]`
- **Full correct**: `[abook (*), khard (*)]`
- **Blocked** (no recipe): abook, khard

### lc0

- **Formula**: `lc0`
- **Current**: `[]`
- **Full correct**: `[eigen (*)]`
- **Blocked** (no recipe): eigen

### ldc

- **Formula**: `ldc`
- **Current**: `["zstd"]`
- **Full correct**: `[llvm@20 (*), zstd]`
- **Blocked** (no recipe): llvm@20

### ldid

- **Formula**: `ldid`
- **Current**: `["libplist"]`
- **Full correct**: `[libplist, openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### ldid-procursus

- **Formula**: `ldid-procursus`
- **Current**: `["libplist"]`
- **Full correct**: `[libplist, openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### ldns

- **Formula**: `ldns`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*), python@3.14 (*)]`
- **Blocked** (no recipe): openssl@3, python@3.14

### lensfun

- **Formula**: `lensfun`
- **Current**: `["gettext", "glib", "libpng"]`
- **Full correct**: `[gettext, glib, libpng, python@3.14 (*)]`
- **Blocked** (no recipe): python@3.14

### leptonica

- **Formula**: `leptonica`
- **Current**: `["giflib", "jpeg-turbo", "libpng"]`
- **Full correct**: `[giflib, jpeg-turbo, libpng, libtiff (*), openjpeg (*), webp (*)]`
- **Blocked** (no recipe): libtiff, openjpeg, webp

### lftp

- **Formula**: `lftp`
- **Current**: `["gettext", "libidn2", "readline"]`
- **Full correct**: `[gettext, libidn2, openssl@3 (*), readline]`
- **Blocked** (no recipe): openssl@3

### libbladerf

- **Formula**: `libbladerf`
- **Current**: `["ncurses"]`
- **Full correct**: `[libusb (*), ncurses]`
- **Blocked** (no recipe): libusb

### libbluray

- **Formula**: `libbluray`
- **Current**: `["fontconfig", "freetype"]`
- **Full correct**: `[fontconfig, freetype, libudfread (*)]`
- **Blocked** (no recipe): libudfread

### libcoap

- **Formula**: `libcoap`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### libcurl

- **Formula**: `curl`
- **Current**: `[]`
- **Full correct**: `[brotli, libnghttp2, libnghttp3, libngtcp2, libssh2, openssl@3 (*), zstd]`
- **Can add now**: brotli, libnghttp2, libnghttp3, libngtcp2, libssh2, zstd
- **Blocked** (no recipe): openssl@3

### libdap

- **Formula**: `libdap`
- **Current**: `["libxml2"]`
- **Full correct**: `[libxml2, openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### libdazzle

- **Formula**: `libdazzle`
- **Current**: `["cairo", "gettext", "glib"]`
- **Full correct**: `[at-spi2-core (*), cairo, gdk-pixbuf (*), gettext, glib, gtk+3 (*), harfbuzz (*), pango (*)]`
- **Blocked** (no recipe): at-spi2-core, gdk-pixbuf, gtk+3, harfbuzz, pango

### libevent

- **Formula**: `libevent`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### libewf

- **Formula**: `libewf`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### libfido2

- **Formula**: `libfido2`
- **Current**: `[]`
- **Full correct**: `[libcbor (*), openssl@3 (*)]`
- **Blocked** (no recipe): libcbor, openssl@3

### libfreenect

- **Formula**: `libfreenect`
- **Current**: `[]`
- **Full correct**: `[libusb (*)]`
- **Blocked** (no recipe): libusb

### libgeotiff

- **Formula**: `libgeotiff`
- **Current**: `["jpeg-turbo", "proj"]`
- **Full correct**: `[jpeg-turbo, libtiff (*), proj]`
- **Blocked** (no recipe): libtiff

### libgit2

- **Formula**: `libgit2`
- **Current**: `[]`
- **Full correct**: `[libssh2]`
- **Can add now**: libssh2

### libgphoto2

- **Formula**: `libgphoto2`
- **Current**: `["gettext", "jpeg-turbo", "libusb-compat"]`
- **Full correct**: `[gd (*), gettext, jpeg-turbo, libexif (*), libtool (*), libusb (*), libusb-compat]`
- **Blocked** (no recipe): gd, libexif, libtool, libusb

### libgr

- **Formula**: `libgr`
- **Current**: `["cairo", "freetype", "jpeg-turbo", "libpng"]`
- **Full correct**: `[cairo, ffmpeg, freetype, glfw (*), jpeg-turbo, libpng, libtiff (*), pixman (*), qhull (*), qtbase, zeromq (*)]`
- **Can add now**: ffmpeg, qtbase
- **Blocked** (no recipe): glfw, libtiff, pixman, qhull, zeromq

### libgrape-lite

- **Formula**: `libgrape-lite`
- **Current**: `["open-mpi"]`
- **Full correct**: `[gflags (*), glog (*), open-mpi]`
- **Blocked** (no recipe): gflags, glog

### libheif

- **Formula**: `libheif`
- **Current**: `["jpeg-turbo", "libpng"]`
- **Full correct**: `[aom (*), jpeg-turbo, libde265 (*), libpng, libtiff (*), shared-mime-info (*), webp (*), x265]`
- **Can add now**: x265
- **Blocked** (no recipe): aom, libde265, libtiff, shared-mime-info, webp

### libidn2

- **Formula**: `libidn2`
- **Current**: `[]`
- **Full correct**: `[gettext, libunistring (*)]`
- **Can add now**: gettext
- **Blocked** (no recipe): libunistring

### libimobiledevice

- **Formula**: `libimobiledevice`
- **Current**: `["libplist", "libtasn1", "libusbmuxd"]`
- **Full correct**: `[libimobiledevice-glue (*), libplist, libtasn1, libtatsu (*), libusbmuxd, openssl@3 (*)]`
- **Blocked** (no recipe): libimobiledevice-glue, libtatsu, openssl@3

### libirecovery

- **Formula**: `libirecovery`
- **Current**: `["libplist"]`
- **Full correct**: `[libimobiledevice-glue (*), libplist]`
- **Blocked** (no recipe): libimobiledevice-glue

### libiscsi

- **Formula**: `libiscsi`
- **Current**: `[]`
- **Full correct**: `[cunit (*)]`
- **Blocked** (no recipe): cunit

### libjwt

- **Formula**: `libjwt`
- **Current**: `["gnutls"]`
- **Full correct**: `[gnutls, jansson (*), openssl@3 (*)]`
- **Blocked** (no recipe): jansson, openssl@3

### libmatio

- **Formula**: `libmatio`
- **Current**: `[]`
- **Full correct**: `[hdf5 (*)]`
- **Blocked** (no recipe): hdf5

### libngtcp2

- **Formula**: `libngtcp2`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### libopenmpt

- **Formula**: `libopenmpt`
- **Current**: `["libsndfile"]`
- **Full correct**: `[flac (*), libogg (*), libsndfile, libvorbis (*), mpg123 (*), portaudio (*)]`
- **Blocked** (no recipe): flac, libogg, libvorbis, mpg123, portaudio

### libosinfo

- **Formula**: `libosinfo`
- **Current**: `["gettext", "glib"]`
- **Full correct**: `[gettext, glib, libsoup (*), osinfo-db (*), usb.ids (*)]`
- **Blocked** (no recipe): libsoup, osinfo-db, usb.ids

### libpaho-mqtt

- **Formula**: `libpaho-mqtt`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### libpsl

- **Formula**: `libpsl`
- **Current**: `["libidn2"]`
- **Full correct**: `[libidn2, libunistring (*)]`
- **Blocked** (no recipe): libunistring

### libqalculate

- **Formula**: `libqalculate`
- **Current**: `["gettext", "gmp", "readline"]`
- **Full correct**: `[gettext, gmp, gnuplot (*), mpfr (*), readline]`
- **Blocked** (no recipe): gnuplot, mpfr

### librasterlite2

- **Formula**: `librasterlite2`
- **Current**: `["cairo", "fontconfig", "freetype", "geos", "giflib", "jpeg-turbo", "libgeotiff", "libpng", "libxml2", "proj", "sqlite", "xz", "zstd"]`
- **Full correct**: `[cairo, fontconfig, freetype, freexl (*), geos, giflib, jpeg-turbo, libgeotiff, libpng, librttopo (*), libspatialite (*), libtiff (*), libxml2, lz4 (*), minizip (*), openjpeg (*), pixman (*), proj, sqlite, webp (*), xz, zstd]`
- **Blocked** (no recipe): freexl, librttopo, libspatialite, libtiff, lz4, minizip, openjpeg, pixman, webp

### librealsense

- **Formula**: `librealsense`
- **Current**: `[]`
- **Full correct**: `[glfw (*), libusb (*)]`
- **Blocked** (no recipe): glfw, libusb

### librime

- **Formula**: `librime`
- **Current**: `["capnp", "lua"]`
- **Full correct**: `[capnp, gflags (*), glog (*), leveldb (*), lua, marisa (*), opencc (*), yaml-cpp (*)]`
- **Blocked** (no recipe): gflags, glog, leveldb, marisa, opencc, yaml-cpp

### librist

- **Formula**: `librist`
- **Current**: `[]`
- **Full correct**: `[cjson (*), libmicrohttpd (*), mbedtls@3 (*)]`
- **Blocked** (no recipe): cjson, libmicrohttpd, mbedtls@3

### librtlsdr

- **Formula**: `librtlsdr`
- **Current**: `[]`
- **Full correct**: `[libusb (*)]`
- **Blocked** (no recipe): libusb

### libsndfile

- **Formula**: `libsndfile`
- **Current**: `[]`
- **Full correct**: `[flac (*), lame (*), libogg (*), libvorbis (*), mpg123 (*), opus (*)]`
- **Blocked** (no recipe): flac, lame, libogg, libvorbis, mpg123, opus

### libssh2

- **Formula**: `libssh2`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### libtrace

- **Formula**: `libtrace`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*), wandio (*)]`
- **Blocked** (no recipe): openssl@3, wandio

### libusb-compat

- **Formula**: `libusb-compat`
- **Current**: `[]`
- **Full correct**: `[libusb (*)]`
- **Blocked** (no recipe): libusb

### libusbmuxd

- **Formula**: `libusbmuxd`
- **Current**: `["libplist"]`
- **Full correct**: `[libimobiledevice-glue (*), libplist]`
- **Blocked** (no recipe): libimobiledevice-glue

### libxc

- **Formula**: `libxc`
- **Current**: `[]`
- **Full correct**: `[gcc (*)]`
- **Blocked** (no recipe): gcc

### libxml2

- **Formula**: `libxml2`
- **Current**: `[]`
- **Full correct**: `[readline]`
- **Can add now**: readline

### libxmlsec1

- **Formula**: `libxmlsec1`
- **Current**: `["gnutls", "libxml2"]`
- **Full correct**: `[gnutls, libxml2, openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### libxpm

- **Formula**: `libxpm`
- **Current**: `["gettext"]`
- **Full correct**: `[gettext, libx11 (*)]`
- **Blocked** (no recipe): libx11

### lighttpd

- **Formula**: `lighttpd`
- **Current**: `["pcre2"]`
- **Full correct**: `[openldap (*), openssl@3 (*), pcre2]`
- **Blocked** (no recipe): openldap, openssl@3

### limesuite

- **Formula**: `limesuite`
- **Current**: `[]`
- **Full correct**: `[fltk (*), gnuplot (*), libusb (*), soapysdr (*)]`
- **Blocked** (no recipe): fltk, gnuplot, libusb, soapysdr

### little-cms2

- **Formula**: `little-cms2`
- **Current**: `["jpeg-turbo"]`
- **Full correct**: `[jpeg-turbo, libtiff (*)]`
- **Blocked** (no recipe): libtiff

### lld

- **Formula**: `lld`
- **Current**: `["zstd"]`
- **Full correct**: `[llvm (*), zstd]`
- **Blocked** (no recipe): llvm

### llgo

- **Formula**: `llgo`
- **Current**: `["bdw-gc", "pkgconf"]`
- **Full correct**: `[bdw-gc, go@1.24 (*), libuv (*), lld@19 (*), llvm@19 (*), openssl@3 (*), pkgconf]`
- **Blocked** (no recipe): go@1.24, libuv, lld@19, llvm@19, openssl@3

### lnav

- **Formula**: `lnav`
- **Current**: `["pcre2", "sqlite"]`
- **Full correct**: `[libarchive (*), libunistring (*), pcre2, sqlite]`
- **Blocked** (no recipe): libarchive, libunistring

### logstalgia

- **Formula**: `logstalgia`
- **Current**: `["freetype", "glew", "libpng", "pcre2", "sdl2"]`
- **Full correct**: `[boost (*), freetype, glew, libpng, pcre2, sdl2, sdl2_image (*)]`
- **Blocked** (no recipe): boost, sdl2_image

### lsdvd

- **Formula**: `lsdvd`
- **Current**: `["libxml2"]`
- **Full correct**: `[libdvdcss (*), libdvdread (*), libxml2]`
- **Blocked** (no recipe): libdvdcss, libdvdread

### ltex-ls

- **Formula**: `ltex-ls`
- **Current**: `[]`
- **Full correct**: `[openjdk@21 (*)]`
- **Blocked** (no recipe): openjdk@21

### macpine

- **Formula**: `macpine`
- **Current**: `[]`
- **Full correct**: `[qemu (*)]`
- **Blocked** (no recipe): qemu

### mailutils

- **Formula**: `mailutils`
- **Current**: `["gdbm", "gettext", "gnutls", "readline"]`
- **Full correct**: `[gdbm, gettext, gnutls, gsasl (*), libtool (*), libunistring (*), readline]`
- **Blocked** (no recipe): gsasl, libtool, libunistring

### makedepend

- **Formula**: `makedepend`
- **Current**: `[]`
- **Full correct**: `[util-macros (*), xorgproto (*)]`
- **Blocked** (no recipe): util-macros, xorgproto

### man-db

- **Formula**: `man-db`
- **Current**: `["groff"]`
- **Full correct**: `[groff, libpipeline (*)]`
- **Blocked** (no recipe): libpipeline

### mapserver

- **Formula**: `mapserver`
- **Current**: `["cairo", "freetype", "geos", "giflib", "jpeg-turbo", "libpng", "libxml2", "pcre2", "proj"]`
- **Full correct**: `[cairo, fcgi (*), freetype, gdal (*), geos, giflib, jpeg-turbo, libpng, libpq (*), libxml2, pcre2, proj, protobuf-c, python@3.14 (*)]`
- **Can add now**: protobuf-c
- **Blocked** (no recipe): fcgi, gdal, libpq, python@3.14

### mariadb-connector-c

- **Formula**: `mariadb-connector-c`
- **Current**: `["zstd"]`
- **Full correct**: `[openssl@3 (*), zstd]`
- **Blocked** (no recipe): openssl@3

### mbpoll

- **Formula**: `mbpoll`
- **Current**: `[]`
- **Full correct**: `[libmodbus (*)]`
- **Blocked** (no recipe): libmodbus

### mender-artifact

- **Formula**: `mender-artifact`
- **Current**: `["e2fsprogs"]`
- **Full correct**: `[dosfstools (*), e2fsprogs, mtools (*), openssl@3 (*)]`
- **Blocked** (no recipe): dosfstools, mtools, openssl@3

### mender-cli

- **Formula**: `mender-cli`
- **Current**: `["xz"]`
- **Full correct**: `[openssl@3 (*), xz]`
- **Blocked** (no recipe): openssl@3

### metals

- **Formula**: `metals`
- **Current**: `[]`
- **Full correct**: `[openjdk (*)]`
- **Blocked** (no recipe): openjdk

### mgba

- **Formula**: `mgba`
- **Current**: `["libpng", "lua", "sdl2", "sqlite"]`
- **Full correct**: `[ffmpeg, libepoxy (*), libpng, libzip (*), lua, qt@5 (*), sdl2, sqlite]`
- **Can add now**: ffmpeg
- **Blocked** (no recipe): libepoxy, libzip, qt@5

### micromamba

- **Formula**: `micromamba`
- **Current**: `["xz", "zstd"]`
- **Full correct**: `[fmt (*), libarchive (*), libsolv (*), lz4 (*), openssl@3 (*), reproc (*), simdjson (*), xz, yaml-cpp (*), zstd]`
- **Blocked** (no recipe): fmt, libarchive, libsolv, lz4, openssl@3, reproc, simdjson, yaml-cpp

### midnight-commander

- **Formula**: `midnight-commander`
- **Current**: `["gettext", "glib", "libssh2"]`
- **Full correct**: `[diffutils (*), gettext, glib, libssh2, openssl@3 (*), s-lang]`
- **Can add now**: s-lang
- **Blocked** (no recipe): diffutils, openssl@3

### mikutter

- **Formula**: `mikutter`
- **Current**: `["cairo", "fontconfig", "freetype", "gettext", "glib", "ruby"]`
- **Full correct**: `[at-spi2-core (*), cairo, fontconfig, freetype, gdk-pixbuf (*), gettext, glib, gobject-introspection (*), gtk+3 (*), harfbuzz (*), pango (*), ruby, terminal-notifier (*)]`
- **Blocked** (no recipe): at-spi2-core, gdk-pixbuf, gobject-introspection, gtk+3, harfbuzz, pango, terminal-notifier

### min-lang

- **Formula**: `min-lang`
- **Current**: `["pcre2"]`
- **Full correct**: `[nim (*), openssl@3 (*), pcre2]`
- **Blocked** (no recipe): nim, openssl@3

### minimal-racket

- **Formula**: `minimal-racket`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### minipro

- **Formula**: `minipro`
- **Current**: `[]`
- **Full correct**: `[libusb (*), srecord]`
- **Can add now**: srecord
- **Blocked** (no recipe): libusb

### mlt

- **Formula**: `mlt`
- **Current**: `["fontconfig", "freetype", "gettext", "glib", "sdl2"]`
- **Full correct**: `[ffmpeg, fftw (*), fontconfig, freetype, frei0r (*), gdk-pixbuf (*), gettext, glib, harfbuzz (*), libdv (*), libexif (*), libsamplerate (*), libvidstab (*), libvorbis (*), opencv (*), pango (*), qt5compat (*), qtbase, qtsvg (*), rubberband (*), sdl2, sox (*)]`
- **Can add now**: ffmpeg, qtbase
- **Blocked** (no recipe): fftw, frei0r, gdk-pixbuf, harfbuzz, libdv, libexif, libsamplerate, libvidstab, libvorbis, opencv, pango, qt5compat, qtsvg, rubberband, sox

### mmseqs2

- **Formula**: `mmseqs2`
- **Current**: `[]`
- **Full correct**: `[libomp (*), wget (*)]`
- **Blocked** (no recipe): libomp, wget

### moarvm

- **Formula**: `moarvm`
- **Current**: `["mimalloc", "zstd"]`
- **Full correct**: `[libtommath (*), libuv (*), mimalloc, zstd]`
- **Blocked** (no recipe): libtommath, libuv

### mongo-c-driver

- **Formula**: `mongo-c-driver`
- **Current**: `["zstd"]`
- **Full correct**: `[openssl@3 (*), zstd]`
- **Blocked** (no recipe): openssl@3

### mrbayes

- **Formula**: `mrbayes`
- **Current**: `["open-mpi"]`
- **Full correct**: `[beagle (*), open-mpi]`
- **Blocked** (no recipe): beagle

### msc-generator

- **Formula**: `msc-generator`
- **Current**: `["cairo", "fontconfig", "libpng", "sdl2"]`
- **Full correct**: `[cairo, fontconfig, gcc (*), glpk (*), graphviz (*), libpng, sdl2, tinyxml2 (*)]`
- **Blocked** (no recipe): gcc, glpk, graphviz, tinyxml2

### msitools

- **Formula**: `msitools`
- **Current**: `["gettext", "glib", "libgsf"]`
- **Full correct**: `[gcab (*), gettext, glib, libgsf]`
- **Blocked** (no recipe): gcab

### musikcube

- **Formula**: `musikcube`
- **Current**: `["gnutls", "libopenmpt", "ncurses"]`
- **Full correct**: `[ffmpeg, game-music-emu (*), gnutls, lame (*), libev (*), libmicrohttpd (*), libopenmpt, mpg123 (*), ncurses, openssl@3 (*), portaudio (*), taglib (*)]`
- **Can add now**: ffmpeg
- **Blocked** (no recipe): game-music-emu, lame, libev, libmicrohttpd, mpg123, openssl@3, portaudio, taglib

### mutt

- **Formula**: `mutt`
- **Current**: `["gettext", "libgpg-error", "libidn2", "ncurses"]`
- **Full correct**: `[gettext, gpgme (*), libgpg-error, libidn2, libunistring (*), ncurses, openssl@3 (*), tokyo-cabinet (*)]`
- **Blocked** (no recipe): gpgme, libunistring, openssl@3, tokyo-cabinet

### mydumper

- **Formula**: `mydumper`
- **Current**: `["glib", "mariadb-connector-c", "pcre2"]`
- **Full correct**: `[glib, mariadb-connector-c, openssl@3 (*), pcre2]`
- **Blocked** (no recipe): openssl@3

### mysql-client

- **Formula**: `mysql-client`
- **Current**: `["libfido2", "zstd"]`
- **Full correct**: `[libfido2, openssl@3 (*), zlib-ng-compat (*), zstd]`
- **Blocked** (no recipe): openssl@3, zlib-ng-compat

### navidrome

- **Formula**: `navidrome`
- **Current**: `[]`
- **Full correct**: `[ffmpeg, taglib (*)]`
- **Can add now**: ffmpeg
- **Blocked** (no recipe): taglib

### ncmpcpp

- **Formula**: `ncmpcpp`
- **Current**: `["ncurses", "readline"]`
- **Full correct**: `[boost (*), fftw (*), icu4c@78 (*), libmpdclient (*), ncurses, readline, taglib (*)]`
- **Blocked** (no recipe): boost, fftw, icu4c@78, libmpdclient, taglib

### neomutt

- **Formula**: `neomutt`
- **Current**: `["gettext", "libgpg-error", "libidn2", "lua", "ncurses", "notmuch", "pcre2", "sqlite"]`
- **Full correct**: `[gettext, gpgme (*), libgpg-error, libiconv (*), libidn2, lmdb (*), lua, ncurses, notmuch, openssl@3 (*), pcre2, sqlite, tokyo-cabinet (*)]`
- **Blocked** (no recipe): gpgme, libiconv, lmdb, openssl@3, tokyo-cabinet

### neovim-qt

- **Formula**: `neovim-qt`
- **Current**: `[]`
- **Full correct**: `[msgpack (*), neovim (*), qtbase, qtsvg (*)]`
- **Can add now**: qtbase
- **Blocked** (no recipe): msgpack, neovim, qtsvg

### netatalk

- **Formula**: `netatalk`
- **Current**: `["libevent", "libgcrypt", "mariadb-connector-c"]`
- **Full correct**: `[berkeley-db@5 (*), bstring (*), cracklib (*), iniparser (*), libevent, libgcrypt, mariadb-connector-c, openldap (*)]`
- **Blocked** (no recipe): berkeley-db@5, bstring, cracklib, iniparser, openldap

### netcdf-fortran

- **Formula**: `netcdf-fortran`
- **Current**: `[]`
- **Full correct**: `[gcc (*), netcdf (*)]`
- **Blocked** (no recipe): gcc, netcdf

### newsboat

- **Formula**: `newsboat`
- **Current**: `["gettext"]`
- **Full correct**: `[gettext, json-c (*)]`
- **Blocked** (no recipe): json-c

### ngrep

- **Formula**: `ngrep`
- **Current**: `["pcre2"]`
- **Full correct**: `[libpcap (*), pcre2]`
- **Blocked** (no recipe): libpcap

### ngspice

- **Formula**: `ngspice`
- **Current**: `["freetype", "readline"]`
- **Full correct**: `[fftw (*), freetype, libice (*), libngspice (*), libsm (*), libx11 (*), libxaw (*), libxext (*), libxmu (*), libxt (*), readline]`
- **Blocked** (no recipe): fftw, libice, libngspice, libsm, libx11, libxaw, libxext, libxmu, libxt

### nip4

- **Formula**: `nip4`
- **Current**: `["cairo", "gettext", "glib", "gsl", "libxml2"]`
- **Full correct**: `[cairo, gdk-pixbuf (*), gettext, glib, graphene (*), gsl, gtk4 (*), hicolor-icon-theme (*), libxml2, pango (*), vips (*)]`
- **Blocked** (no recipe): gdk-pixbuf, graphene, gtk4, hicolor-icon-theme, pango, vips

### notmuch

- **Formula**: `notmuch`
- **Current**: `[]`
- **Full correct**: `[cffi (*), gettext, glib, gmime (*), python@3.14 (*), sfsexp (*), talloc (*), xapian (*)]`
- **Can add now**: gettext, glib
- **Blocked** (no recipe): cffi, gmime, python@3.14, sfsexp, talloc, xapian

### nwchem

- **Formula**: `nwchem`
- **Current**: `["libxc", "open-mpi", "pkgconf"]`
- **Full correct**: `[gcc (*), hwloc (*), libomp (*), libxc, open-mpi, openblas (*), pkgconf, python@3.14 (*), scalapack (*)]`
- **Blocked** (no recipe): gcc, hwloc, libomp, openblas, python@3.14, scalapack

### oath-toolkit

- **Formula**: `oath-toolkit`
- **Current**: `["libxml2", "libxmlsec1"]`
- **Full correct**: `[libxml2, libxmlsec1, openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### ocicl

- **Formula**: `ocicl`
- **Current**: `["zstd"]`
- **Full correct**: `[sbcl, zstd]`
- **Can add now**: sbcl

### omniorb

- **Formula**: `omniorb`
- **Current**: `["zstd"]`
- **Full correct**: `[openssl@3 (*), python@3.14 (*), zstd]`
- **Blocked** (no recipe): openssl@3, python@3.14

### onedrive-cli

- **Formula**: `onedrive-cli`
- **Current**: `["curl", "sqlite"]`
- **Full correct**: `[curl, dbus (*), sqlite, systemd (*)]`
- **Blocked** (no recipe): dbus, systemd

### open-image-denoise

- **Formula**: `open-image-denoise`
- **Current**: `[]`
- **Full correct**: `[tbb (*)]`
- **Blocked** (no recipe): tbb

### open-mpi

- **Formula**: `open-mpi`
- **Current**: `["libevent"]`
- **Full correct**: `[gcc (*), hwloc (*), libevent, pmix (*)]`
- **Blocked** (no recipe): gcc, hwloc, pmix

### open-ocd

- **Formula**: `open-ocd`
- **Current**: `["hidapi"]`
- **Full correct**: `[capstone (*), hidapi, libftdi (*), libusb (*)]`
- **Blocked** (no recipe): capstone, libftdi, libusb

### open-simh

- **Formula**: `open-simh`
- **Current**: `["libpng"]`
- **Full correct**: `[libpng, vde]`
- **Can add now**: vde

### opencoarrays

- **Formula**: `opencoarrays`
- **Current**: `["open-mpi"]`
- **Full correct**: `[gcc@13 (*), open-mpi]`
- **Blocked** (no recipe): gcc@13

### openfortivpn

- **Formula**: `openfortivpn`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### openfpgaloader

- **Formula**: `openfpgaloader`
- **Current**: `[]`
- **Full correct**: `[libftdi (*), libusb (*)]`
- **Blocked** (no recipe): libftdi, libusb

### openjph

- **Formula**: `openjph`
- **Current**: `[]`
- **Full correct**: `[libtiff (*)]`
- **Blocked** (no recipe): libtiff

### openmsx

- **Formula**: `openmsx`
- **Current**: `["freetype", "glew", "libpng", "sdl2"]`
- **Full correct**: `[freetype, glew, libogg (*), libpng, libvorbis (*), sdl2, sdl2_ttf (*), tcl-tk, theora (*)]`
- **Can add now**: tcl-tk
- **Blocked** (no recipe): libogg, libvorbis, sdl2_ttf, theora

### openrtsp

- **Formula**: `openrtsp`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### opensc

- **Formula**: `opensc`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### opensearch-dashboards

- **Formula**: `opensearch-dashboards`
- **Current**: `[]`
- **Full correct**: `[node@22 (*)]`
- **Blocked** (no recipe): node@22

### operator-sdk

- **Formula**: `operator-sdk`
- **Current**: `["go", "libassuan", "libgpg-error"]`
- **Full correct**: `[go, gpgme (*), libassuan, libgpg-error]`
- **Blocked** (no recipe): gpgme

### osm2pgrouting

- **Formula**: `osm2pgrouting`
- **Current**: `[]`
- **Full correct**: `[boost (*), libpq (*), libpqxx (*), pgrouting (*), postgis (*)]`
- **Blocked** (no recipe): boost, libpq, libpqxx, pgrouting, postgis

### osm2pgsql

- **Formula**: `osm2pgsql`
- **Current**: `["luajit", "proj"]`
- **Full correct**: `[libpq (*), luajit, proj]`
- **Blocked** (no recipe): libpq

### osmcoastline

- **Formula**: `osmcoastline`
- **Current**: `["geos"]`
- **Full correct**: `[gdal (*), geos, libspatialite (*), lz4 (*)]`
- **Blocked** (no recipe): gdal, libspatialite, lz4

### osmium-tool

- **Formula**: `osmium-tool`
- **Current**: `[]`
- **Full correct**: `[boost (*), lz4 (*)]`
- **Blocked** (no recipe): boost, lz4

### osslsigncode

- **Formula**: `osslsigncode`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### pandoc-crossref

- **Formula**: `pandoc-crossref`
- **Current**: `["gmp"]`
- **Full correct**: `[gmp, pandoc (*)]`
- **Blocked** (no recipe): pandoc

### pandoc-plot

- **Formula**: `pandoc-plot`
- **Current**: `["gmp"]`
- **Full correct**: `[gmp, pandoc (*)]`
- **Blocked** (no recipe): pandoc

### par2

- **Formula**: `par2`
- **Current**: `[]`
- **Full correct**: `[libomp (*)]`
- **Blocked** (no recipe): libomp

### partio

- **Formula**: `partio`
- **Current**: `[]`
- **Full correct**: `[python@3.14 (*)]`
- **Blocked** (no recipe): python@3.14

### pc6001vx

- **Formula**: `pc6001vx`
- **Current**: `["gettext", "sdl2"]`
- **Full correct**: `[ffmpeg, gettext, qtbase, qtmultimedia (*), sdl2]`
- **Can add now**: ffmpeg, qtbase
- **Blocked** (no recipe): qtmultimedia

### pcb2gcode

- **Formula**: `pcb2gcode`
- **Current**: `["cairo", "gerbv", "gettext", "glib"]`
- **Full correct**: `[at-spi2-core (*), boost (*), cairo, gdk-pixbuf (*), gerbv, gettext, glib, gtk+ (*), harfbuzz (*), pango (*)]`
- **Blocked** (no recipe): at-spi2-core, boost, gdk-pixbuf, gtk+, harfbuzz, pango

### pcl

- **Formula**: `pcl`
- **Current**: `["freetype", "glew", "libpng"]`
- **Full correct**: `[boost (*), cjson (*), eigen (*), flann (*), freetype, glew, libomp (*), libpcap (*), libpng, libusb (*), lz4 (*), qhull (*), qtbase, vtk (*)]`
- **Can add now**: qtbase
- **Blocked** (no recipe): boost, cjson, eigen, flann, libomp, libpcap, libusb, lz4, qhull, vtk

### pdf2svg

- **Formula**: `pdf2svg`
- **Current**: `["cairo", "gettext", "glib"]`
- **Full correct**: `[cairo, gettext, glib, poppler (*)]`
- **Blocked** (no recipe): poppler

### pdfgrep

- **Formula**: `pdfgrep`
- **Current**: `["libgcrypt", "libgpg-error", "pcre2"]`
- **Full correct**: `[libgcrypt, libgpg-error, pcre2, poppler (*)]`
- **Blocked** (no recipe): poppler

### pdfpc

- **Formula**: `pdfpc`
- **Current**: `["cairo", "gettext", "glib", "json-glib"]`
- **Full correct**: `[at-spi2-core (*), cairo, discount (*), gdk-pixbuf (*), gettext, glib, gstreamer (*), gtk+3 (*), harfbuzz (*), json-glib, libgee (*), librsvg (*), libx11 (*), pango (*), poppler (*)]`
- **Blocked** (no recipe): at-spi2-core, discount, gdk-pixbuf, gstreamer, gtk+3, harfbuzz, libgee, librsvg, libx11, pango, poppler

### pdftk-java

- **Formula**: `pdftk-java`
- **Current**: `[]`
- **Full correct**: `[openjdk (*)]`
- **Blocked** (no recipe): openjdk

### pdftoipe

- **Formula**: `pdftoipe`
- **Current**: `[]`
- **Full correct**: `[poppler (*)]`
- **Blocked** (no recipe): poppler

### pdnsrec

- **Formula**: `pdnsrec`
- **Current**: `["lua"]`
- **Full correct**: `[boost (*), lua, openssl@3 (*)]`
- **Blocked** (no recipe): boost, openssl@3

### percona-server

- **Formula**: `percona-server`
- **Current**: `["abseil", "libfido2", "zstd"]`
- **Full correct**: `[abseil, icu4c@78 (*), libfido2, lz4 (*), openldap (*), openssl@3 (*), protobuf (*), zlib-ng-compat (*), zstd]`
- **Blocked** (no recipe): icu4c@78, lz4, openldap, openssl@3, protobuf, zlib-ng-compat

### percona-toolkit

- **Formula**: `percona-toolkit`
- **Current**: `[]`
- **Full correct**: `[perl-dbd-mysql (*)]`
- **Blocked** (no recipe): perl-dbd-mysql

### percona-xtrabackup

- **Formula**: `percona-xtrabackup`
- **Current**: `["libgcrypt", "zstd"]`
- **Full correct**: `[icu4c@78 (*), libev (*), libgcrypt, lz4 (*), openssl@3 (*), protobuf (*), zlib-ng-compat (*), zstd]`
- **Blocked** (no recipe): icu4c@78, libev, lz4, openssl@3, protobuf, zlib-ng-compat

### pgbackrest

- **Formula**: `pgbackrest`
- **Current**: `["libssh2", "libyaml", "zstd"]`
- **Full correct**: `[libpq (*), libssh2, libyaml, lz4 (*), openssl@3 (*), zstd]`
- **Blocked** (no recipe): libpq, lz4, openssl@3

### phoneinfoga

- **Formula**: `phoneinfoga`
- **Current**: `[]`
- **Full correct**: `[node (*)]`
- **Blocked** (no recipe): node

### php

- **Formula**: `php`
- **Current**: `["apr", "apr-util", "curl", "freetds", "gettext", "gmp", "oniguruma", "pcre2", "sqlite", "tidy-html5", "unixodbc"]`
- **Full correct**: `[apr, apr-util, argon2 (*), autoconf (*), curl, freetds, gd (*), gettext, gmp, icu4c@78 (*), libpq (*), libsodium (*), libzip (*), net-snmp (*), oniguruma, openldap (*), openssl@3 (*), pcre2, sqlite, tidy-html5, unixodbc]`
- **Blocked** (no recipe): argon2, autoconf, gd, icu4c@78, libpq, libsodium, libzip, net-snmp, openldap, openssl@3

### phpbrew

- **Formula**: `phpbrew`
- **Current**: `[]`
- **Full correct**: `[php@8.4 (*)]`
- **Blocked** (no recipe): php@8.4

### pianobar

- **Formula**: `pianobar`
- **Current**: `["libgcrypt"]`
- **Full correct**: `[ffmpeg, json-c (*), libao (*), libgcrypt]`
- **Can add now**: ffmpeg
- **Blocked** (no recipe): json-c, libao

### pianod

- **Formula**: `pianod`
- **Current**: `["gettext", "glib", "gnutls"]`
- **Full correct**: `[gettext, glib, gnutls, gstreamer (*), taglib (*)]`
- **Blocked** (no recipe): gstreamer, taglib

### pioneers

- **Formula**: `pioneers`
- **Current**: `["cairo", "gettext", "glib"]`
- **Full correct**: `[at-spi2-core (*), cairo, gdk-pixbuf (*), gettext, glib, gtk+3 (*), harfbuzz (*), librsvg (*), pango (*)]`
- **Blocked** (no recipe): at-spi2-core, gdk-pixbuf, gtk+3, harfbuzz, librsvg, pango

### pixlet

- **Formula**: `pixlet`
- **Current**: `[]`
- **Full correct**: `[webp (*)]`
- **Blocked** (no recipe): webp

### pixz

- **Formula**: `pixz`
- **Current**: `["xz"]`
- **Full correct**: `[libarchive (*), xz]`
- **Blocked** (no recipe): libarchive

### pjproject

- **Formula**: `pjproject`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### pkcs11-tools

- **Formula**: `pkcs11-tools`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### pkgx

- **Formula**: `pkgx`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### pngcrush

- **Formula**: `pngcrush`
- **Current**: `[]`
- **Full correct**: `[libpng]`
- **Can add now**: libpng

### polynote

- **Formula**: `polynote`
- **Current**: `["numpy"]`
- **Full correct**: `[numpy, openjdk (*), python@3.14 (*)]`
- **Blocked** (no recipe): openjdk, python@3.14

### poppler-qt5

- **Formula**: `poppler-qt5`
- **Current**: `["cairo", "fontconfig", "freetype", "gettext", "glib", "jpeg-turbo", "libassuan", "libpng", "little-cms2", "nspr"]`
- **Full correct**: `[cairo, fontconfig, freetype, gettext, glib, gpgme (*), gpgmepp (*), jpeg-turbo, libassuan, libpng, libtiff (*), little-cms2, nspr, nss (*), openjpeg (*), qt@5 (*)]`
- **Blocked** (no recipe): gpgme, gpgmepp, libtiff, nss, openjpeg, qt@5

### postgres-language-server

- **Formula**: `postgres-language-server`
- **Current**: `[]`
- **Full correct**: `[libpg_query (*)]`
- **Blocked** (no recipe): libpg_query

### povray

- **Formula**: `povray`
- **Current**: `["jpeg-turbo", "libpng"]`
- **Full correct**: `[boost (*), imath (*), jpeg-turbo, libpng, libtiff (*), openexr (*)]`
- **Blocked** (no recipe): boost, imath, libtiff, openexr

### powerman

- **Formula**: `powerman`
- **Current**: `[]`
- **Full correct**: `[jansson (*)]`
- **Blocked** (no recipe): jansson

### ppsspp

- **Formula**: `ppsspp`
- **Current**: `["sdl2", "snappy", "zstd"]`
- **Full correct**: `[libzip (*), miniupnpc (*), molten-vk (*), sdl2, snappy, zstd]`
- **Blocked** (no recipe): libzip, miniupnpc, molten-vk

### pqiv

- **Formula**: `pqiv`
- **Current**: `["cairo", "gettext", "glib"]`
- **Full correct**: `[at-spi2-core (*), cairo, gdk-pixbuf (*), gettext, glib, gtk+3 (*), harfbuzz (*), imagemagick (*), libarchive (*), libspectre (*), pango (*), poppler (*), webp (*)]`
- **Blocked** (no recipe): at-spi2-core, gdk-pixbuf, gtk+3, harfbuzz, imagemagick, libarchive, libspectre, pango, poppler, webp

### prog8

- **Formula**: `prog8`
- **Current**: `[]`
- **Full correct**: `[openjdk (*), tass64 (*)]`
- **Blocked** (no recipe): openjdk, tass64

### proj

- **Formula**: `proj`
- **Current**: `[]`
- **Full correct**: `[libtiff (*)]`
- **Blocked** (no recipe): libtiff

### protobuf-c

- **Formula**: `protobuf-c`
- **Current**: `["abseil"]`
- **Full correct**: `[abseil, protobuf (*)]`
- **Blocked** (no recipe): protobuf

### protoc-gen-doc

- **Formula**: `protoc-gen-doc`
- **Current**: `[]`
- **Full correct**: `[protobuf (*)]`
- **Blocked** (no recipe): protobuf

### protoc-gen-go

- **Formula**: `protoc-gen-go`
- **Current**: `[]`
- **Full correct**: `[protobuf (*)]`
- **Blocked** (no recipe): protobuf

### protoc-gen-go-grpc

- **Formula**: `protoc-gen-go-grpc`
- **Current**: `[]`
- **Full correct**: `[protobuf (*)]`
- **Blocked** (no recipe): protobuf

### protoc-gen-grpc-java

- **Formula**: `protoc-gen-grpc-java`
- **Current**: `["abseil"]`
- **Full correct**: `[abseil, protobuf (*)]`
- **Blocked** (no recipe): protobuf

### protoc-gen-grpc-swift

- **Formula**: `protoc-gen-grpc-swift`
- **Current**: `[]`
- **Full correct**: `[protobuf (*), swift-protobuf]`
- **Can add now**: swift-protobuf
- **Blocked** (no recipe): protobuf

### pstoedit

- **Formula**: `pstoedit`
- **Current**: `[]`
- **Full correct**: `[gd (*), ghostscript (*), imagemagick (*), libzip (*), plotutils (*)]`
- **Blocked** (no recipe): gd, ghostscript, imagemagick, libzip, plotutils

### pure-ftpd

- **Formula**: `pure-ftpd`
- **Current**: `[]`
- **Full correct**: `[libsodium (*), openssl@3 (*)]`
- **Blocked** (no recipe): libsodium, openssl@3

### pyenv-virtualenv

- **Formula**: `pyenv-virtualenv`
- **Current**: `[]`
- **Full correct**: `[coreutils (*), pyenv (*)]`
- **Blocked** (no recipe): coreutils, pyenv

### pyqt

- **Formula**: `pyqt`
- **Current**: `["qtbase", "qtdeclarative", "qtquick3d", "qtshadertools", "qttools"]`
- **Full correct**: `[python@3.14 (*), qt3d (*), qtbase, qtcharts (*), qtconnectivity (*), qtdatavis3d (*), qtdeclarative, qtmultimedia (*), qtnetworkauth (*), qtpositioning (*), qtquick3d, qtremoteobjects (*), qtscxml (*), qtsensors (*), qtserialport (*), qtshadertools, qtspeech (*), qtsvg (*), qttools, qtwebchannel (*), qtwebengine (*), qtwebsockets (*)]`
- **Blocked** (no recipe): python@3.14, qt3d, qtcharts, qtconnectivity, qtdatavis3d, qtmultimedia, qtnetworkauth, qtpositioning, qtremoteobjects, qtscxml, qtsensors, qtserialport, qtspeech, qtsvg, qtwebchannel, qtwebengine, qtwebsockets

### python-freethreading

- **Formula**: `python-freethreading`
- **Current**: `["sqlite", "xz", "zstd"]`
- **Full correct**: `[mpdecimal (*), openssl@3 (*), sqlite, xz, zstd]`
- **Blocked** (no recipe): mpdecimal, openssl@3

### python-tabulate

- **Formula**: `python-tabulate`
- **Current**: `[]`
- **Full correct**: `[python@3.14 (*)]`
- **Blocked** (no recipe): python@3.14

### qalculate-gtk

- **Formula**: `qalculate-gtk`
- **Current**: `["cairo", "gettext", "glib", "libqalculate"]`
- **Full correct**: `[adwaita-icon-theme (*), at-spi2-core (*), cairo, gdk-pixbuf (*), gettext, glib, gtk+3 (*), harfbuzz (*), libqalculate, pango (*)]`
- **Blocked** (no recipe): adwaita-icon-theme, at-spi2-core, gdk-pixbuf, gtk+3, harfbuzz, pango

### qalculate-qt

- **Formula**: `qalculate-qt`
- **Current**: `["gmp", "libqalculate", "qtbase"]`
- **Full correct**: `[gmp, libqalculate, mpfr (*), qtbase]`
- **Blocked** (no recipe): mpfr

### qca

- **Formula**: `qca`
- **Current**: `["ca-certificates", "libgcrypt", "nspr", "qtbase"]`
- **Full correct**: `[botan (*), ca-certificates, gnupg (*), libgcrypt, nspr, nss (*), openssl@3 (*), pkcs11-helper (*), qt5compat (*), qtbase]`
- **Blocked** (no recipe): botan, gnupg, nss, openssl@3, pkcs11-helper, qt5compat

### qcachegrind

- **Formula**: `qcachegrind`
- **Current**: `["qtbase"]`
- **Full correct**: `[graphviz (*), qtbase]`
- **Blocked** (no recipe): graphviz

### qdmr

- **Formula**: `qdmr`
- **Current**: `["qtbase", "qttools"]`
- **Full correct**: `[librsvg (*), libusb (*), qtbase, qtpositioning (*), qtserialport (*), qttools, yaml-cpp (*)]`
- **Blocked** (no recipe): librsvg, libusb, qtpositioning, qtserialport, yaml-cpp

### qmmp

- **Formula**: `qmmp`
- **Current**: `["faad2", "gettext", "glib", "jack", "libcdio", "libsndfile", "qtbase"]`
- **Full correct**: `[faad2, ffmpeg, flac (*), game-music-emu (*), gettext, glib, jack, libarchive (*), libbs2b (*), libcddb (*), libcdio, libcdio-paranoia (*), libmms (*), libmodplug (*), libogg (*), libsamplerate (*), libshout (*), libsndfile, libsoxr (*), libvorbis (*), libxmp (*), mad (*), mpg123 (*), mplayer (*), musepack (*), opus (*), opusfile (*), projectm (*), pulseaudio (*), qtbase, qtmultimedia (*), taglib (*), wavpack (*), wildmidi (*)]`
- **Can add now**: ffmpeg
- **Blocked** (no recipe): flac, game-music-emu, libarchive, libbs2b, libcddb, libcdio-paranoia, libmms, libmodplug, libogg, libsamplerate, libshout, libsoxr, libvorbis, libxmp, mad, mpg123, mplayer, musepack, opus, opusfile, projectm, pulseaudio, qtmultimedia, taglib, wavpack, wildmidi

### qtbase

- **Formula**: `qtbase`
- **Current**: `["brotli", "freetype", "glib", "jpeg-turbo", "libpng", "pcre2", "zstd"]`
- **Full correct**: `[brotli, dbus (*), double-conversion (*), freetype, glib, harfbuzz (*), icu4c@78 (*), jpeg-turbo, libb2 (*), libpng, md4c (*), openssl@3 (*), pcre2, zstd]`
- **Blocked** (no recipe): dbus, double-conversion, harfbuzz, icu4c@78, libb2, md4c, openssl@3

### qtdeclarative

- **Formula**: `qtdeclarative`
- **Current**: `["qtbase"]`
- **Full correct**: `[qtbase, qtsvg (*)]`
- **Blocked** (no recipe): qtsvg

### qtquick3d

- **Formula**: `qtquick3d`
- **Current**: `["qtbase", "qtdeclarative", "qtshadertools"]`
- **Full correct**: `[assimp (*), qtbase, qtdeclarative, qtquicktimeline (*), qtshadertools]`
- **Blocked** (no recipe): assimp, qtquicktimeline

### qtserialbus

- **Formula**: `qtserialbus`
- **Current**: `["qtbase"]`
- **Full correct**: `[qtbase, qtserialport (*)]`
- **Blocked** (no recipe): qtserialport

### qttools

- **Formula**: `qttools`
- **Current**: `["qtbase", "qtdeclarative", "zstd"]`
- **Full correct**: `[gumbo-parser (*), litehtml (*), qtbase, qtdeclarative, zstd]`
- **Blocked** (no recipe): gumbo-parser, litehtml

### rabbitmq-c

- **Formula**: `rabbitmq-c`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*), popt (*)]`
- **Blocked** (no recipe): openssl@3, popt

### rakudo-star

- **Formula**: `rakudo-star`
- **Current**: `["mimalloc", "readline", "zstd"]`
- **Full correct**: `[libtommath (*), libuv (*), mimalloc, openssl@3 (*), readline, zstd]`
- **Blocked** (no recipe): libtommath, libuv, openssl@3

### rattler-build

- **Formula**: `rattler-build`
- **Current**: `["xz"]`
- **Full correct**: `[openssl@3 (*), xz]`
- **Blocked** (no recipe): openssl@3

### rdfind

- **Formula**: `rdfind`
- **Current**: `[]`
- **Full correct**: `[nettle (*)]`
- **Blocked** (no recipe): nettle

### readsb

- **Formula**: `readsb`
- **Current**: `["librtlsdr", "zstd"]`
- **Full correct**: `[librtlsdr, libusb (*), zstd]`
- **Blocked** (no recipe): libusb

### rebar3

- **Formula**: `rebar3`
- **Current**: `[]`
- **Full correct**: `[erlang (*)]`
- **Blocked** (no recipe): erlang

### reprepro

- **Formula**: `reprepro`
- **Current**: `["libgpg-error", "xz", "zstd"]`
- **Full correct**: `[berkeley-db@5 (*), gcc (*), gpgme (*), libarchive (*), libgpg-error, xz, zstd]`
- **Blocked** (no recipe): berkeley-db@5, gcc, gpgme, libarchive

### retdec

- **Formula**: `retdec`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*), python@3.14 (*)]`
- **Blocked** (no recipe): openssl@3, python@3.14

### riscv64-elf-gcc

- **Formula**: `riscv64-elf-gcc`
- **Current**: `["gmp", "riscv64-elf-binutils", "zstd"]`
- **Full correct**: `[gmp, libmpc (*), mpfr (*), riscv64-elf-binutils, zstd]`
- **Blocked** (no recipe): libmpc, mpfr

### riscv64-elf-gdb

- **Formula**: `riscv64-elf-gdb`
- **Current**: `["gmp", "ncurses", "readline", "xz", "zstd"]`
- **Full correct**: `[gmp, mpfr (*), ncurses, python@3.14 (*), readline, xz, zstd]`
- **Blocked** (no recipe): mpfr, python@3.14

### rizin

- **Formula**: `rizin`
- **Current**: `["pcre2", "tree-sitter", "xz", "zstd"]`
- **Full correct**: `[capstone (*), libmagic (*), libzip (*), lz4 (*), openssl@3 (*), pcre2, tree-sitter, xxhash (*), xz, zstd]`
- **Blocked** (no recipe): capstone, libmagic, libzip, lz4, openssl@3, xxhash

### rlwrap

- **Formula**: `rlwrap`
- **Current**: `["readline"]`
- **Full correct**: `[libptytty (*), readline]`
- **Blocked** (no recipe): libptytty

### rocq

- **Formula**: `rocq`
- **Current**: `["gmp"]`
- **Full correct**: `[gmp, ocaml (*), ocaml-findlib (*), ocaml-zarith (*)]`
- **Blocked** (no recipe): ocaml, ocaml-findlib, ocaml-zarith

### rom-tools

- **Formula**: `rom-tools`
- **Current**: `["sdl2", "zstd"]`
- **Full correct**: `[flac (*), sdl3 (*), utf8proc (*), zstd]`
- **Blocked** (no recipe): flac, sdl3, utf8proc

### rosa-cli

- **Formula**: `rosa-cli`
- **Current**: `[]`
- **Full correct**: `[awscli (*)]`
- **Blocked** (no recipe): awscli

### rpm2cpio

- **Formula**: `rpm2cpio`
- **Current**: `["xz"]`
- **Full correct**: `[libarchive (*), xz]`
- **Blocked** (no recipe): libarchive

### rsgain

- **Formula**: `rsgain`
- **Current**: `[]`
- **Full correct**: `[ffmpeg, fmt (*), inih (*), libebur128 (*), taglib (*)]`
- **Can add now**: ffmpeg
- **Blocked** (no recipe): fmt, inih, libebur128, taglib

### rtabmap

- **Formula**: `rtabmap`
- **Current**: `["freetype", "glew", "libfreenect", "libpng", "librealsense", "pcl", "qtbase", "sqlite"]`
- **Full correct**: `[boost (*), flann (*), freetype, g2o (*), glew, libfreenect, libomp (*), libpcap (*), libpng, librealsense, lz4 (*), octomap (*), opencv (*), pcl, pdal (*), qhull (*), qtbase, qtsvg (*), sqlite, vtk (*)]`
- **Blocked** (no recipe): boost, flann, g2o, libomp, libpcap, lz4, octomap, opencv, pdal, qhull, qtsvg, vtk

### rtl-433

- **Formula**: `rtl_433`
- **Current**: `["librtlsdr"]`
- **Full correct**: `[librtlsdr, libusb (*), openssl@3 (*)]`
- **Blocked** (no recipe): libusb, openssl@3

### rtmpdump

- **Formula**: `rtmpdump`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### rtorrent

- **Formula**: `rtorrent`
- **Current**: `[]`
- **Full correct**: `[libtorrent-rakshasa (*), xmlrpc-c (*)]`
- **Blocked** (no recipe): libtorrent-rakshasa, xmlrpc-c

### rxvt-unicode

- **Formula**: `rxvt-unicode`
- **Current**: `["fontconfig", "freetype"]`
- **Full correct**: `[fontconfig, freetype, libx11 (*), libxext (*), libxft (*), libxmu (*), libxrender (*), libxt (*)]`
- **Blocked** (no recipe): libx11, libxext, libxft, libxmu, libxrender, libxt

### samba

- **Formula**: `samba`
- **Current**: `["gettext", "gnutls", "libtasn1", "readline"]`
- **Full correct**: `[gettext, gnutls, icu4c@78 (*), krb5 (*), libtasn1, libxcrypt (*), lmdb (*), openssl@3 (*), popt (*), readline, talloc (*), tdb (*), tevent (*)]`
- **Blocked** (no recipe): icu4c@78, krb5, libxcrypt, lmdb, openssl@3, popt, talloc, tdb, tevent

### sambamba

- **Formula**: `sambamba`
- **Current**: `[]`
- **Full correct**: `[lz4 (*)]`
- **Blocked** (no recipe): lz4

### samtools

- **Formula**: `samtools`
- **Current**: `[]`
- **Full correct**: `[htslib (*)]`
- **Blocked** (no recipe): htslib

### sane-backends

- **Formula**: `sane-backends`
- **Current**: `["jpeg-turbo", "libpng"]`
- **Full correct**: `[jpeg-turbo, libpng, libtiff (*), libusb (*), net-snmp (*)]`
- **Blocked** (no recipe): libtiff, libusb, net-snmp

### sc-im

- **Formula**: `sc-im`
- **Current**: `["libxml2", "lua", "ncurses"]`
- **Full correct**: `[libxls (*), libxlsxwriter (*), libxml2, libzip (*), lua, ncurses]`
- **Blocked** (no recipe): libxls, libxlsxwriter, libzip

### scummvm

- **Formula**: `scummvm`
- **Current**: `["faad2", "fluid-synth", "freetype", "fribidi", "giflib", "jpeg-turbo", "libopenmpt", "libpng", "sdl2"]`
- **Full correct**: `[a52dec (*), faad2, flac (*), fluid-synth, freetype, fribidi, giflib, jpeg-turbo, libmpeg2 (*), libogg (*), libopenmpt, libpng, libvorbis (*), libvpx (*), mad (*), musepack (*), sdl2, theora (*)]`
- **Blocked** (no recipe): a52dec, flac, libmpeg2, libogg, libvorbis, libvpx, mad, musepack, theora

### scummvm-tools

- **Formula**: `scummvm-tools`
- **Current**: `["freetype", "libpng"]`
- **Full correct**: `[boost (*), flac (*), freetype, libogg (*), libpng, libvorbis (*), mad (*), wxwidgets]`
- **Can add now**: wxwidgets
- **Blocked** (no recipe): boost, flac, libogg, libvorbis, mad

### sdcc

- **Formula**: `sdcc`
- **Current**: `["readline", "zstd"]`
- **Full correct**: `[gputils (*), readline, zstd]`
- **Blocked** (no recipe): gputils

### sentry-cli

- **Formula**: `sentry-cli`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### shairport-sync

- **Formula**: `shairport-sync`
- **Current**: `[]`
- **Full correct**: `[libao (*), libconfig (*), libdaemon (*), libsoxr (*), openssl@3 (*), popt (*), pulseaudio (*)]`
- **Blocked** (no recipe): libao, libconfig, libdaemon, libsoxr, openssl@3, popt, pulseaudio

### sigrok-cli

- **Formula**: `sigrok-cli`
- **Current**: `["gettext", "glib"]`
- **Full correct**: `[gettext, glib, libsigrok (*), libsigrokdecode (*)]`
- **Blocked** (no recipe): libsigrok, libsigrokdecode

### sipsak

- **Formula**: `sipsak`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### siril

- **Formula**: `siril`
- **Current**: `["cairo", "cfitsio", "exiv2", "gettext", "glib", "gsl", "jpeg-turbo", "jpeg-xl", "json-glib", "libgit2", "libheif", "libpng", "little-cms2"]`
- **Full correct**: `[cairo, cfitsio, exiv2, ffmpeg, ffms2 (*), fftw (*), gdk-pixbuf (*), gettext, glib, gnuplot (*), gsl, gtk+3 (*), gtksourceview4 (*), healpix (*), jpeg-turbo, jpeg-xl, json-glib, libgit2, libheif, libomp (*), libpng, libraw (*), librsvg (*), libtiff (*), little-cms2, netpbm (*), opencv (*), pango (*), wcslib (*), yyjson (*)]`
- **Can add now**: ffmpeg
- **Blocked** (no recipe): ffms2, fftw, gdk-pixbuf, gnuplot, gtk+3, gtksourceview4, healpix, libomp, libraw, librsvg, libtiff, netpbm, opencv, pango, wcslib, yyjson

### skopeo

- **Formula**: `skopeo`
- **Current**: `[]`
- **Full correct**: `[gpgme (*)]`
- **Blocked** (no recipe): gpgme

### sleuthkit

- **Formula**: `sleuthkit`
- **Current**: `["afflib", "libewf", "sqlite"]`
- **Full correct**: `[afflib, libewf, libpq (*), openjdk (*), openssl@3 (*), sqlite]`
- **Blocked** (no recipe): libpq, openjdk, openssl@3

### sngrep

- **Formula**: `sngrep`
- **Current**: `["ncurses"]`
- **Full correct**: `[ncurses, openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### socat

- **Formula**: `socat`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### sofia-sip

- **Formula**: `sofia-sip`
- **Current**: `["gettext", "glib"]`
- **Full correct**: `[gettext, glib, openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### softhsm

- **Formula**: `softhsm`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### sox-ng

- **Formula**: `sox_ng`
- **Current**: `["libpng", "libsndfile"]`
- **Full correct**: `[flac (*), lame (*), libogg (*), libpng, libsndfile, libvorbis (*), mad (*), opus (*), opusfile (*), wavpack (*)]`
- **Blocked** (no recipe): flac, lame, libogg, libvorbis, mad, opus, opusfile, wavpack

### spatialite

- **Formula**: `libspatialite`
- **Current**: `[]`
- **Full correct**: `[freexl (*), geos, librttopo (*), libxml2, minizip (*), proj, sqlite]`
- **Can add now**: geos, libxml2, proj, sqlite
- **Blocked** (no recipe): freexl, librttopo, minizip

### spatialite-gui

- **Formula**: `spatialite-gui`
- **Current**: `["geos", "librasterlite2", "libxml2", "proj", "sqlite", "xz", "zstd"]`
- **Full correct**: `[freexl (*), geos, libpq (*), librasterlite2, librttopo (*), libspatialite (*), libtiff (*), libxlsxwriter (*), libxml2, lz4 (*), minizip (*), openjpeg (*), proj, sqlite, virtualpg (*), webp (*), wxwidgets@3.2 (*), xz, zstd]`
- **Blocked** (no recipe): freexl, libpq, librttopo, libspatialite, libtiff, libxlsxwriter, lz4, minizip, openjpeg, virtualpg, webp, wxwidgets@3.2

### spatialite-tools

- **Formula**: `spatialite-tools`
- **Current**: `["geos", "libxml2", "proj", "readline", "sqlite"]`
- **Full correct**: `[freexl (*), geos, librttopo (*), libspatialite (*), libxml2, minizip (*), proj, readline, readosm (*), sqlite]`
- **Blocked** (no recipe): freexl, librttopo, libspatialite, minizip, readosm

### speex

- **Formula**: `speex`
- **Current**: `[]`
- **Full correct**: `[libogg (*)]`
- **Blocked** (no recipe): libogg

### spice-gtk

- **Formula**: `spice-gtk`
- **Current**: `["cairo", "gettext", "glib", "jpeg-turbo", "json-glib"]`
- **Full correct**: `[at-spi2-core (*), cairo, gdk-pixbuf (*), gettext, glib, gobject-introspection (*), gstreamer (*), gtk+3 (*), harfbuzz (*), jpeg-turbo, json-glib, libepoxy (*), libsoup (*), libusb (*), libx11 (*), lz4 (*), openssl@3 (*), opus (*), pango (*), phodav (*), pixman (*), spice-protocol (*), usbredir]`
- **Can add now**: usbredir
- **Blocked** (no recipe): at-spi2-core, gdk-pixbuf, gobject-introspection, gstreamer, gtk+3, harfbuzz, libepoxy, libsoup, libusb, libx11, lz4, openssl@3, opus, pango, phodav, pixman, spice-protocol

### spirv-llvm-translator

- **Formula**: `spirv-llvm-translator`
- **Current**: `[]`
- **Full correct**: `[llvm@21 (*)]`
- **Blocked** (no recipe): llvm@21

### sqlcipher

- **Formula**: `sqlcipher`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### sqlite

- **Formula**: `sqlite`
- **Current**: `[]`
- **Full correct**: `[readline]`
- **Can add now**: readline

### sqlite-analyzer

- **Formula**: `sqlite-analyzer`
- **Current**: `[]`
- **Full correct**: `[libtommath (*), tcl-tk]`
- **Can add now**: tcl-tk
- **Blocked** (no recipe): libtommath

### squashfs

- **Formula**: `squashfs`
- **Current**: `["xz", "zstd"]`
- **Full correct**: `[lz4 (*), lzo (*), xz, zstd]`
- **Blocked** (no recipe): lz4, lzo

### sratoolkit

- **Formula**: `sratoolkit`
- **Current**: `[]`
- **Full correct**: `[hdf5 (*)]`
- **Blocked** (no recipe): hdf5

### stlink

- **Formula**: `stlink`
- **Current**: `[]`
- **Full correct**: `[libusb (*)]`
- **Blocked** (no recipe): libusb

### strongswan

- **Formula**: `strongswan`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### suite-sparse

- **Formula**: `suite-sparse`
- **Current**: `["gmp"]`
- **Full correct**: `[gcc (*), gmp, libomp (*), mpfr (*)]`
- **Blocked** (no recipe): gcc, libomp, mpfr

### supertux

- **Formula**: `supertux`
- **Current**: `["freetype", "glew", "libpng", "physfs", "sdl2"]`
- **Full correct**: `[boost (*), freetype, glew, glm (*), libogg (*), libpng, libvorbis (*), physfs, sdl2, sdl2_image (*)]`
- **Blocked** (no recipe): boost, glm, libogg, libvorbis, sdl2_image

### swift-protobuf

- **Formula**: `swift-protobuf`
- **Current**: `[]`
- **Full correct**: `[protobuf (*)]`
- **Blocked** (no recipe): protobuf

### swtpm

- **Formula**: `swtpm`
- **Current**: `["gettext", "glib", "gmp", "gnutls", "json-glib", "libtasn1"]`
- **Full correct**: `[gettext, glib, gmp, gnutls, json-glib, libtasn1, libtpms (*), openssl@3 (*)]`
- **Blocked** (no recipe): libtpms, openssl@3

### synergy-core

- **Formula**: `synergy-core`
- **Current**: `["qtbase"]`
- **Full correct**: `[openssl@3 (*), qtbase]`
- **Blocked** (no recipe): openssl@3

### synfig

- **Formula**: `synfig`
- **Current**: `["cairo", "ffmpeg", "fontconfig", "freetype", "fribidi", "gettext", "glib", "libpng", "little-cms2", "mlt"]`
- **Full correct**: `[cairo, etl (*), ffmpeg, fftw (*), fontconfig, freetype, fribidi, gettext, glib, glibmm@2.66 (*), harfbuzz (*), imagemagick (*), imath (*), liblqr (*), libmng (*), libomp (*), libpng, libsigc++@2 (*), libtool (*), libxml++ (*), libzip (*), little-cms2, mlt, openexr (*), pango (*)]`
- **Blocked** (no recipe): etl, fftw, glibmm@2.66, harfbuzz, imagemagick, imath, liblqr, libmng, libomp, libsigc++@2, libtool, libxml++, libzip, openexr, pango

### syslog-ng

- **Formula**: `syslog-ng`
- **Current**: `["abseil", "gettext", "glib", "libpaho-mqtt", "mongo-c-driver", "pcre2", "rabbitmq-c"]`
- **Full correct**: `[abseil, gettext, glib, grpc (*), hiredis (*), ivykis (*), json-c (*), libdbi (*), libmaxminddb (*), libnet (*), libpaho-mqtt, librdkafka (*), mongo-c-driver, net-snmp (*), openssl@3 (*), pcre2, protobuf (*), python@3.14 (*), rabbitmq-c, riemann-client (*)]`
- **Blocked** (no recipe): grpc, hiredis, ivykis, json-c, libdbi, libmaxminddb, libnet, librdkafka, net-snmp, openssl@3, protobuf, python@3.14, riemann-client

### tailwindcss-language-server

- **Formula**: `tailwindcss-language-server`
- **Current**: `[]`
- **Full correct**: `[node (*)]`
- **Blocked** (no recipe): node

### tcl-tk

- **Formula**: `tcl-tk`
- **Current**: `[]`
- **Full correct**: `[libtommath (*), openssl@3 (*)]`
- **Blocked** (no recipe): libtommath, openssl@3

### tcpdump

- **Formula**: `tcpdump`
- **Current**: `[]`
- **Full correct**: `[libpcap (*), openssl@3 (*)]`
- **Blocked** (no recipe): libpcap, openssl@3

### tcpflow

- **Formula**: `tcpflow`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### termscp

- **Formula**: `termscp`
- **Current**: `["samba"]`
- **Full correct**: `[openssl@3 (*), samba]`
- **Blocked** (no recipe): openssl@3

### terraform-provider-libvirt

- **Formula**: `terraform-provider-libvirt`
- **Current**: `[]`
- **Full correct**: `[libvirt (*)]`
- **Blocked** (no recipe): libvirt

### thors-anvil

- **Formula**: `thors-anvil`
- **Current**: `["libevent", "libyaml", "snappy"]`
- **Full correct**: `[boost (*), libevent, libyaml, magic_enum (*), openssl@3 (*), snappy]`
- **Blocked** (no recipe): boost, magic_enum, openssl@3

### tiger-vnc

- **Formula**: `tiger-vnc`
- **Current**: `["gettext", "gmp", "gnutls", "jpeg-turbo"]`
- **Full correct**: `[fltk@1.3 (*), gettext, gmp, gnutls, jpeg-turbo, nettle (*), pixman (*)]`
- **Blocked** (no recipe): fltk@1.3, nettle, pixman

### tmuxai

- **Formula**: `tmuxai`
- **Current**: `[]`
- **Full correct**: `[tmux (*)]`
- **Blocked** (no recipe): tmux

### transmission-cli

- **Formula**: `transmission-cli`
- **Current**: `["libevent"]`
- **Full correct**: `[libevent, miniupnpc (*)]`
- **Blocked** (no recipe): miniupnpc

### tronbyt-server

- **Formula**: `tronbyt-server`
- **Current**: `[]`
- **Full correct**: `[webp (*)]`
- **Blocked** (no recipe): webp

### tsduck

- **Formula**: `tsduck`
- **Current**: `["librist"]`
- **Full correct**: `[librist, libvatek (*), openssl@3 (*), srt (*)]`
- **Blocked** (no recipe): libvatek, openssl@3, srt

### ttyd

- **Formula**: `ttyd`
- **Current**: `["libevent"]`
- **Full correct**: `[json-c (*), libevent, libuv (*), libwebsockets (*), openssl@3 (*)]`
- **Blocked** (no recipe): json-c, libuv, libwebsockets, openssl@3

### u-boot-tools

- **Formula**: `u-boot-tools`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### ugrep

- **Formula**: `ugrep`
- **Current**: `["brotli", "pcre2", "xz", "zstd"]`
- **Full correct**: `[brotli, lz4 (*), pcre2, xz, zstd]`
- **Blocked** (no recipe): lz4

### uhubctl

- **Formula**: `uhubctl`
- **Current**: `[]`
- **Full correct**: `[libusb (*)]`
- **Blocked** (no recipe): libusb

### umoci

- **Formula**: `umoci`
- **Current**: `[]`
- **Full correct**: `[gpgme (*)]`
- **Blocked** (no recipe): gpgme

### undercutf1

- **Formula**: `undercutf1`
- **Current**: `["ffmpeg", "fontconfig"]`
- **Full correct**: `[dotnet (*), ffmpeg, fontconfig, mpg123 (*)]`
- **Blocked** (no recipe): dotnet, mpg123

### universal-ctags

- **Formula**: `universal-ctags`
- **Current**: `["libyaml", "pcre2"]`
- **Full correct**: `[jansson (*), libyaml, pcre2]`
- **Blocked** (no recipe): jansson

### unixodbc

- **Formula**: `unixodbc`
- **Current**: `[]`
- **Full correct**: `[libtool (*)]`
- **Blocked** (no recipe): libtool

### unshield

- **Formula**: `unshield`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### urdfdom

- **Formula**: `urdfdom`
- **Current**: `[]`
- **Full correct**: `[console_bridge (*), tinyxml2 (*), urdfdom_headers (*)]`
- **Blocked** (no recipe): console_bridge, tinyxml2, urdfdom_headers

### usbredir

- **Formula**: `usbredir`
- **Current**: `["glib"]`
- **Full correct**: `[glib, libusb (*)]`
- **Blocked** (no recipe): libusb

### uuu

- **Formula**: `uuu`
- **Current**: `["zstd"]`
- **Full correct**: `[libusb (*), libzip (*), openssl@3 (*), tinyxml2 (*), zstd]`
- **Blocked** (no recipe): libusb, libzip, openssl@3, tinyxml2

### vcftools

- **Formula**: `vcftools`
- **Current**: `[]`
- **Full correct**: `[htslib (*)]`
- **Blocked** (no recipe): htslib

### vcluster

- **Formula**: `vcluster`
- **Current**: `["kubernetes-cli"]`
- **Full correct**: `[helm@3 (*), kubernetes-cli]`
- **Blocked** (no recipe): helm@3

### vgmstream

- **Formula**: `vgmstream`
- **Current**: `["ffmpeg", "speex"]`
- **Full correct**: `[ffmpeg, libao (*), libogg (*), libvorbis (*), mpg123 (*), speex]`
- **Blocked** (no recipe): libao, libogg, libvorbis, mpg123

### visp

- **Formula**: `visp`
- **Current**: `["glew", "gsl", "jpeg-turbo", "libpng", "pcl", "qtbase"]`
- **Full correct**: `[boost (*), eigen (*), flann (*), glew, gsl, jpeg-turbo, libdc1394 (*), libomp (*), libpcap (*), libpng, lz4 (*), openblas (*), opencv (*), pcl, qhull (*), qtbase, vtk (*), zbar (*)]`
- **Blocked** (no recipe): boost, eigen, flann, libdc1394, libomp, libpcap, lz4, openblas, opencv, qhull, vtk, zbar

### vitess

- **Formula**: `vitess`
- **Current**: `[]`
- **Full correct**: `[etcd (*)]`
- **Blocked** (no recipe): etcd

### vnstat

- **Formula**: `vnstat`
- **Current**: `[]`
- **Full correct**: `[gd (*)]`
- **Blocked** (no recipe): gd

### vorbis-tools

- **Formula**: `vorbis-tools`
- **Current**: `[]`
- **Full correct**: `[flac (*), libao (*), libogg (*), libvorbis (*)]`
- **Blocked** (no recipe): flac, libao, libogg, libvorbis

### votca

- **Formula**: `votca`
- **Current**: `["gromacs", "libxc"]`
- **Full correct**: `[boost (*), eigen (*), fftw (*), gromacs, hdf5 (*), libecpint (*), libint (*), libomp (*), libxc]`
- **Blocked** (no recipe): boost, eigen, fftw, hdf5, libecpint, libint, libomp

### vulkan-tools

- **Formula**: `vulkan-tools`
- **Current**: `["vulkan-loader"]`
- **Full correct**: `[glslang (*), molten-vk (*), vulkan-headers (*), vulkan-loader]`
- **Blocked** (no recipe): glslang, molten-vk, vulkan-headers

### wget2

- **Formula**: `wget2`
- **Current**: `["brotli", "gettext", "gnutls", "libidn2", "libnghttp2", "libpsl", "pcre2", "xz", "zstd"]`
- **Full correct**: `[brotli, gettext, gnutls, gpgme (*), libidn2, libnghttp2, libpsl, lzlib (*), pcre2, xz, zstd]`
- **Blocked** (no recipe): gpgme, lzlib

### widelands

- **Formula**: `widelands`
- **Current**: `["gettext", "glew", "libpng", "lua", "sdl2"]`
- **Full correct**: `[gettext, glew, icu4c@78 (*), libpng, lua, minizip (*), sdl2, sdl2_image (*), sdl2_mixer (*), sdl2_ttf (*)]`
- **Blocked** (no recipe): icu4c@78, minizip, sdl2_image, sdl2_mixer, sdl2_ttf

### wireshark

- **Formula**: `wireshark`
- **Current**: `["c-ares", "glib", "gnutls", "libgcrypt", "libgpg-error", "libnghttp2", "libnghttp3", "lua", "pcre2", "zstd"]`
- **Full correct**: `[c-ares, glib, gnutls, libgcrypt, libgpg-error, libmaxminddb (*), libnghttp2, libnghttp3, libsmi (*), libssh (*), lua, lz4 (*), pcre2, speexdsp (*), zstd]`
- **Blocked** (no recipe): libmaxminddb, libsmi, libssh, lz4, speexdsp

### wuppiefuzz

- **Formula**: `wuppiefuzz`
- **Current**: `[]`
- **Full correct**: `[z3 (*)]`
- **Blocked** (no recipe): z3

### wxmaxima

- **Formula**: `wxmaxima`
- **Current**: `["wxwidgets"]`
- **Full correct**: `[maxima (*), wxwidgets]`
- **Blocked** (no recipe): maxima

### wxwidgets

- **Formula**: `wxwidgets`
- **Current**: `["jpeg-turbo", "libpng", "pcre2"]`
- **Full correct**: `[jpeg-turbo, libpng, libtiff (*), pcre2, webp (*)]`
- **Blocked** (no recipe): libtiff, webp

### x11vnc

- **Formula**: `x11vnc`
- **Current**: `[]`
- **Full correct**: `[libvncserver (*), openssl@3 (*)]`
- **Blocked** (no recipe): libvncserver, openssl@3

### x3270

- **Formula**: `x3270`
- **Current**: `["readline"]`
- **Full correct**: `[openssl@3 (*), readline, tcl-tk@8 (*)]`
- **Blocked** (no recipe): openssl@3, tcl-tk@8

### xeyes

- **Formula**: `xeyes`
- **Current**: `[]`
- **Full correct**: `[libx11 (*), libxcb (*), libxext (*), libxi (*), libxmu (*), libxrender (*), libxt (*)]`
- **Blocked** (no recipe): libx11, libxcb, libxext, libxi, libxmu, libxrender, libxt

### xfig

- **Formula**: `xfig`
- **Current**: `["fig2dev", "fontconfig", "freetype", "jpeg-turbo", "libpng", "libxpm"]`
- **Full correct**: `[fig2dev, fontconfig, freetype, ghostscript (*), jpeg-turbo, libpng, libtiff (*), libx11 (*), libxaw3d (*), libxft (*), libxpm, libxt (*)]`
- **Blocked** (no recipe): ghostscript, libtiff, libx11, libxaw3d, libxft, libxt

### xmlto

- **Formula**: `xmlto`
- **Current**: `["gnu-getopt"]`
- **Full correct**: `[docbook (*), docbook-xsl (*), gnu-getopt]`
- **Blocked** (no recipe): docbook, docbook-xsl

### xorg-server

- **Formula**: `xorg-server`
- **Current**: `[]`
- **Full correct**: `[libapplewm (*), libx11 (*), libxau (*), libxcb (*), libxdmcp (*), libxext (*), libxfixes (*), libxfont2 (*), mesa (*), pixman (*), xauth (*), xcb-util (*), xcb-util-image (*), xcb-util-keysyms (*), xcb-util-renderutil (*), xcb-util-wm (*), xkbcomp (*), xkeyboard-config (*)]`
- **Blocked** (no recipe): libapplewm, libx11, libxau, libxcb, libxdmcp, libxext, libxfixes, libxfont2, mesa, pixman, xauth, xcb-util, xcb-util-image, xcb-util-keysyms, xcb-util-renderutil, xcb-util-wm, xkbcomp, xkeyboard-config

### xsel

- **Formula**: `xsel`
- **Current**: `[]`
- **Full correct**: `[libx11 (*)]`
- **Blocked** (no recipe): libx11

### yubico-piv-tool

- **Formula**: `yubico-piv-tool`
- **Current**: `[]`
- **Full correct**: `[openssl@3 (*)]`
- **Blocked** (no recipe): openssl@3

### zeek

- **Formula**: `zeek`
- **Current**: `["c-ares"]`
- **Full correct**: `[c-ares, libmaxminddb (*), libuv (*), node@24 (*), openssl@3 (*), python@3.14 (*), zeromq (*)]`
- **Blocked** (no recipe): libmaxminddb, libuv, node@24, openssl@3, python@3.14, zeromq

### zstd

- **Formula**: `zstd`
- **Current**: `[]`
- **Full correct**: `[lz4 (*), xz]`
- **Can add now**: xz
- **Blocked** (no recipe): lz4

## Impact Analysis

### Creating the top 20 library recipes would unblock:

| Rank | Dep Name | Direct Dependents | Cumulative Unique Recipes Touched |
|------|----------|-------------------|-----------------------------------|
| 1 | openssl@3 | 137 | 137 |
| 2 | python@3.14 | 36 | 166 |
| 3 | pango | 35 | 197 |
| 4 | gdk-pixbuf | 34 | 201 |
| 5 | harfbuzz | 27 | 203 |
| 6 | libtiff | 26 | 222 |
| 7 | libusb | 25 | 243 |
| 8 | gtk+3 | 24 | 243 |
| 9 | boost | 23 | 257 |
| 10 | at-spi2-core | 22 | 257 |
| 11 | lz4 | 21 | 264 |
| 12 | openjdk | 18 | 278 |
| 13 | libvorbis | 17 | 289 |
| 14 | libx11 | 17 | 298 |
| 15 | libomp | 16 | 305 |
| 16 | libogg | 15 | 306 |
| 17 | gcc | 14 | 313 |
| 18 | libarchive | 14 | 318 |
| 19 | fftw | 13 | 320 |
| 20 | webp | 13 | 323 |

Creating just these 20 library recipes would address at least one missing dep
in 323 of the 521 affected recipes (61%).

Of those, 142 recipes would be **fully unblocked** (all their missing deps
would have recipes after creating those 20).
