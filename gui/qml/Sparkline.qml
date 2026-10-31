import QtQuick
import org.kde.kirigami as Kirigami

// A small line chart of recent samples; scales to its own maximum unless maxValue is set.
Canvas {
    id: spark

    property var values: []
    property real maxValue: 0
    property color lineColor: Kirigami.Theme.highlightColor

    implicitHeight: Kirigami.Units.gridUnit * 2.5
    onValuesChanged: requestPaint()
    onWidthChanged: requestPaint()

    onPaint: {
        const ctx = getContext("2d")
        ctx.reset()
        const n = values.length
        if (n < 2)
            return
        const max = maxValue > 0 ? maxValue : Math.max(1, ...values)
        const step = width / (n - 1)
        const y = v => height - 1 - (height - 2) * Math.min(1, v / max)
        ctx.beginPath()
        ctx.moveTo(0, y(values[0]))
        for (let i = 1; i < n; i++)
            ctx.lineTo(i * step, y(values[i]))
        ctx.lineWidth = 1.5
        ctx.strokeStyle = lineColor
        ctx.stroke()
        ctx.lineTo(width, height)
        ctx.lineTo(0, height)
        ctx.closePath()
        ctx.fillStyle = Qt.rgba(lineColor.r, lineColor.g, lineColor.b, 0.12)
        ctx.fill()
    }
}
