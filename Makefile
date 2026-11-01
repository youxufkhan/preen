VERSION ?= 0.1.2
PREFIX  ?= /usr
B       := build

QT         := Qt6Widgets Qt6Quick Qt6QuickControls2 Qt6Qml Qt6Gui Qt6Core
QT_LIBEXEC ?= $(shell pkg-config --variable=libexecdir Qt6Core)
MOC        ?= $(QT_LIBEXEC)/moc
RCC        ?= $(QT_LIBEXEC)/rcc
CXXFLAGS   ?= -O2
CXXFLAGS   += -std=c++17 -fPIC -Wall $(shell pkg-config --cflags $(QT)) -DPREEN_VERSION='"$(VERSION)"'
LDLIBS     ?= $(shell pkg-config --libs $(QT))

all: $(B)/preen $(B)/preen-gui

$(B)/preen: $(wildcard *.go internal/*/*.go) go.mod
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $@ .

$(B)/moc_engine.cpp: gui/engine.h
	@mkdir -p $(B)
	$(MOC) $< -o $@

$(B)/qrc_gui.cpp: gui/gui.qrc gui/preen.svg $(wildcard gui/qml/*)
	@mkdir -p $(B)
	$(RCC) $< -o $@

$(B)/preen-gui: gui/main.cpp $(B)/moc_engine.cpp $(B)/qrc_gui.cpp
	$(CXX) $(CXXFLAGS) -Igui $^ -o $@ $(LDFLAGS) $(LDLIBS)

test:
	go test ./...

install: all
	install -Dm755 $(B)/preen $(DESTDIR)$(PREFIX)/bin/preen
	install -Dm755 $(B)/preen-gui $(DESTDIR)$(PREFIX)/bin/preen-gui
	install -Dm644 packaging/preen.desktop $(DESTDIR)$(PREFIX)/share/applications/preen.desktop
	install -Dm644 gui/preen.svg $(DESTDIR)$(PREFIX)/share/icons/hicolor/scalable/apps/preen.svg
	install -Dm644 packaging/org.preen.helper.policy $(DESTDIR)$(PREFIX)/share/polkit-1/actions/org.preen.helper.policy

deb: all
	rm -rf $(B)/deb
	$(MAKE) install DESTDIR=$(B)/deb PREFIX=/usr
	mkdir -p $(B)/deb/DEBIAN
	sed -e 's/@VERSION@/$(VERSION)/' -e "s/@SIZE@/$$(du -sk $(B)/deb/usr | cut -f1)/" packaging/control > $(B)/deb/DEBIAN/control
	dpkg-deb --build --root-owner-group $(B)/deb $(B)/preen_$(VERSION)_amd64.deb

clean:
	rm -rf $(B)

.PHONY: all test install deb clean
