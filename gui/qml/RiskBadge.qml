import QtQuick
import QtQuick.Controls as QQC2
import org.kde.kirigami as Kirigami

// Shown only when an item needs a second look. Safe items carry no badge.
Rectangle {
    property string risk: "safe"

    readonly property string text: risk === "review" ? "Review" : risk === "personal" ? "Personal data" : ""
    readonly property color tone: risk === "personal" ? Kirigami.Theme.negativeTextColor : Kirigami.Theme.neutralTextColor

    visible: text !== ""
    implicitWidth: label.implicitWidth + Kirigami.Units.largeSpacing
    implicitHeight: label.implicitHeight + Kirigami.Units.smallSpacing / 2
    radius: Kirigami.Units.cornerRadius
    color: Qt.rgba(tone.r, tone.g, tone.b, 0.14)

    QQC2.Label {
        id: label
        anchors.centerIn: parent
        text: parent.text
        color: parent.tone
        font: Kirigami.Theme.smallFont
    }
}
