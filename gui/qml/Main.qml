import QtQuick
import QtQuick.Controls as QQC2
import org.kde.kirigami as Kirigami

Kirigami.ApplicationWindow {
    id: root

    title: "Preen"
    width: Kirigami.Units.gridUnit * 64
    height: Kirigami.Units.gridUnit * 42
    minimumWidth: Kirigami.Units.gridUnit * 36
    minimumHeight: Kirigami.Units.gridUnit * 26

    property string current: ""

    // One entry per sidebar item. Scan pages share ScanPage.qml with different settings.
    readonly property var pages: ({
        overview: { url: "OverviewPage.qml", props: {} },
        clean: { url: "ScanPage.qml", props: {
            title: "Clean", command: "clean", applyText: "Clean",
            scanningText: "Measuring caches, logs and old packages…",
            emptyText: "Nothing to clean",
            heroCaption: "can be cleaned", stepText: "locations checked" } },
        uninstall: { url: "UninstallPage.qml", props: {} },
        disk: { url: "DiskPage.qml", props: {} },
        projects: { url: "ScanPage.qml", props: {
            title: "Projects", command: "purge", applyText: "Remove",
            scanningText: "Looking for build folders in your projects…",
            emptyText: "No build folders found",
            heroCaption: "of rebuildable build folders", folders: true, stepText: "steps" } },
        installers: { url: "ScanPage.qml", props: {
            title: "Installers", command: "installer", applyText: "Remove",
            scanningText: "Looking for installer files…",
            emptyText: "No installer files",
            emptyExplanation: "Checked Downloads and Desktop for .deb, .rpm, .AppImage, .iso, .run and similar files.",
            heroCaption: "of installer files" } },
        maintenance: { url: "ScanPage.qml", props: {
            title: "Maintenance", command: "optimize", applyText: "Run",
            scanningText: "Checking system health…",
            emptyText: "Nothing to do",
            heroCaption: "", countMode: true } },
        history: { url: "HistoryPage.qml", props: {} }
    })

    function show(key) {
        const p = pages[key] || pages.overview
        current = pages[key] ? key : "overview"
        pageStack.clear()
        pageStack.push(Qt.resolvedUrl(p.url), p.props)
    }

    pageStack.globalToolBar.style: Kirigami.ApplicationHeaderStyle.ToolBar
    pageStack.defaultColumnWidth: Kirigami.Units.gridUnit * 26

    globalDrawer: Kirigami.GlobalDrawer {
        title: "Preen"
        titleIcon: "qrc:/preen.svg"
        modal: !root.wideScreen
        handleVisible: modal
        width: Kirigami.Units.gridUnit * 12
        actions: [
            Kirigami.Action { text: "Overview"; icon.name: "speedometer-symbolic"; checkable: true; checked: root.current === "overview"; onTriggered: root.show("overview") },
            Kirigami.Action { text: "Clean"; icon.name: "edit-clear-all-symbolic"; checkable: true; checked: root.current === "clean"; onTriggered: root.show("clean") },
            Kirigami.Action { text: "Uninstall"; icon.name: "archive-remove-symbolic"; checkable: true; checked: root.current === "uninstall"; onTriggered: root.show("uninstall") },
            Kirigami.Action { text: "Disk"; icon.name: "drive-harddisk-symbolic"; checkable: true; checked: root.current === "disk"; onTriggered: root.show("disk") },
            Kirigami.Action { text: "Projects"; icon.name: "folder-development-symbolic"; checkable: true; checked: root.current === "projects"; onTriggered: root.show("projects") },
            Kirigami.Action { text: "Installers"; icon.name: "download-symbolic"; checkable: true; checked: root.current === "installers"; onTriggered: root.show("installers") },
            Kirigami.Action { text: "Maintenance"; icon.name: "tools-symbolic"; checkable: true; checked: root.current === "maintenance"; onTriggered: root.show("maintenance") },
            Kirigami.Action { text: "History"; icon.name: "view-history"; checkable: true; checked: root.current === "history"; onTriggered: root.show("history") }
        ]
    }

    Component.onCompleted: show(startPage)
}
