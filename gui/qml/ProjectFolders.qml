import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Dialogs
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Preen 1.0

// The folders Projects searches for build folders (~/.config/preen/purge_paths).
Kirigami.Dialog {
    id: dialog

    signal foldersChanged()

    property var paths: []
    property bool edited: false
    property string error: ""

    title: "Project folders"
    preferredWidth: Kirigami.Units.gridUnit * 30
    standardButtons: Kirigami.Dialog.NoButton
    flatFooterButtons: false

    onOpened: {
        edited = false
        error = ""
        engine.run(["purge", "--paths"])
    }
    onClosed: if (edited) foldersChanged()

    function tilde(p) {
        return p.replace(/^\/home\/[^/]+/, "~")
    }

    Engine {
        id: engine
        onReceived: (e) => { if (e.type === "paths") dialog.paths = e.data || [] }
        onFinished: (code, err) => { if (code !== 0) dialog.error = err }
    }

    customFooterActions: [
        Kirigami.Action {
            text: "Add folder…"
            icon.name: "folder-add"
            onTriggered: picker.open()
        },
        Kirigami.Action {
            text: "Done"
            icon.name: "dialog-ok-apply"
            onTriggered: dialog.close()
        }
    ]

    ColumnLayout {
        spacing: Kirigami.Units.smallSpacing

        QQC2.Label {
            text: "Preen looks for build folders such as node_modules, target and .venv up to six levels inside these folders."
            wrapMode: Text.Wrap
            color: Kirigami.Theme.disabledTextColor
            Layout.fillWidth: true
            Layout.margins: Kirigami.Units.largeSpacing
        }

        Kirigami.InlineMessage {
            visible: dialog.error !== ""
            type: Kirigami.MessageType.Error
            text: dialog.error
            Layout.fillWidth: true
        }

        Repeater {
            model: dialog.paths
            delegate: QQC2.ItemDelegate {
                required property string modelData
                Layout.fillWidth: true
                hoverEnabled: false
                contentItem: RowLayout {
                    spacing: Kirigami.Units.largeSpacing
                    Kirigami.Icon {
                        source: "folder"
                        implicitWidth: Kirigami.Units.iconSizes.smallMedium
                        implicitHeight: Kirigami.Units.iconSizes.smallMedium
                    }
                    QQC2.Label {
                        text: dialog.tilde(modelData)
                        elide: Text.ElideMiddle
                        Layout.fillWidth: true
                    }
                    QQC2.ToolButton {
                        icon.name: "list-remove-symbolic"
                        text: "Remove"
                        display: QQC2.AbstractButton.IconOnly
                        QQC2.ToolTip.text: "Stop searching this folder"
                        QQC2.ToolTip.visible: hovered
                        QQC2.ToolTip.delay: Kirigami.Units.toolTipDelay
                        onClicked: {
                            dialog.edited = true
                            engine.run(["purge", "--remove-path", modelData])
                        }
                    }
                }
            }
        }

        QQC2.Label {
            visible: dialog.paths.length === 0
            text: "No folders yet. Add the folders where you keep code."
            color: Kirigami.Theme.disabledTextColor
            Layout.margins: Kirigami.Units.largeSpacing
        }
    }

    FolderDialog {
        id: picker
        title: "Choose a folder with projects"
        onAccepted: {
            dialog.edited = true
            dialog.error = ""
            engine.run(["purge", "--add-path", decodeURIComponent(selectedFolder.toString().replace(/^file:\/\//, ""))])
        }
    }
}
