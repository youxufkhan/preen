.pragma library

// Same decimal units as the engine's human(): 1 GB = 1000^3 bytes.
function bytes(b) {
    if (b === undefined || b === null || isNaN(b) || b <= 0)
        return "–"
    if (b < 1000)
        return Math.round(b) + " B"
    const units = ["kB", "MB", "GB", "TB", "PB"]
    let i = -1
    do {
        b /= 1000
        i++
    } while (b >= 1000 && i < units.length - 1)
    return b.toFixed(1) + " " + units[i]
}

function rate(b) {
    return b > 0 ? bytes(b) + "/s" : "0 B/s"
}

function uptime(sec) {
    const d = Math.floor(sec / 86400), h = Math.floor(sec % 86400 / 3600), m = Math.floor(sec % 3600 / 60)
    return d > 0 ? d + "d " + h + "h" : h + "h " + m + "m"
}

function plural(n, word) {
    return n === 1 ? "1 " + word : n + " " + word + "s"
}
