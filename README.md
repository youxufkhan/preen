<div align="center">
  <img src="assets/preen-banner.svg" alt="Preen Banner" width="760" />
</div>

<br/>

Clean, uninstall, analyze, and monitor Linux from one application. Preen is built for Ubuntu and Kubuntu and inspired by [Mole](https://github.com/tw93/mole) for macOS.

- **Clean**: removes caches, logs, old snap versions, crash dumps, old kernels, browser and Electron app caches, and developer caches (uv, pnpm, npm, Bun, pip, Gradle, Cargo, Go).
- **Uninstall**: removes apt, snap, Flatpak, and AppImage applications together with residual files. Configuration files remain unless specifically selected.
- **Disk**: inspects disk usage folder by folder and moves items to the Trash.
- **Projects**: removes rebuildable `node_modules`, `target`, `build`, `.venv`, and similar build directories.
- **Installers**: cleans leftover `.deb`, `.rpm`, `.run`, `.AppImage`, and `.iso` files from Downloads and Desktop.
- **Maintenance**: rebuilds font and application menu caches, flushes DNS, configures snap retention, and checks systemd service health.
- **Overview**: reports CPU load, memory distribution, disk throughput, network traffic, battery health, thermal sensors, and active processes.

Every removal is presented for review before execution. Nothing runs in the background.

## Interface and screenshots

| Overview & Telemetry | Clean Scan |
| :---: | :---: |
| <img src="assets/screenshots/01-overview.png" width="460" alt="Preen Overview" /> | <img src="assets/screenshots/02-clean.png" width="460" alt="Preen Clean" /> |
| CPU, memory, disk, network, and thermal telemetry | Reclaimable caches, logs, old kernels, and app leftovers |

| Application Uninstaller | Disk Space Explorer |
| :---: | :---: |
| <img src="assets/screenshots/03-uninstall.png" width="460" alt="Preen Uninstall" /> | <img src="assets/screenshots/04-disk.png" width="460" alt="Preen Disk Analyzer" /> |
| Removes apt, snap, Flatpak, and AppImage packages | Hierarchical folder sizes with Trash support |

| Project Build Folders | Downloaded Installers |
| :---: | :---: |
| <img src="assets/screenshots/05-projects.png" width="460" alt="Preen Projects Purge" /> | <img src="assets/screenshots/06-installers.png" width="460" alt="Preen Installers" /> |
| Cleans rebuildable `.next`, `.venv`, and `node_modules` folders | Finds leftover `.deb`, `.rpm`, and `.AppImage` packages |

| System Maintenance | Command Line Interface |
| :---: | :---: |
| <img src="assets/screenshots/07-maintenance.png" width="460" alt="Preen Maintenance" /> | <img src="assets/screenshots/00-cli.png" width="460" alt="Preen CLI" /> |
| Cache rebuilds, DNS flushes, and service diagnostics | Terminal dashboard with human-readable and `--json` outputs |

## How Preen compares

| Capability | Preen | BleachBit | btop | CCleaner (Windows) | Mole (macOS) |
|---|:---:|:---:|:---:|:---:|:---:|
| **Platform and interface** | | | | | |
| Runs on Linux | Yes | Yes | Yes | No | No |
| Desktop app | KDE (Kirigami) | GTK | Terminal | Win32 / Qt | App |
| Command line with JSON | Yes | No JSON | No | Partial | Yes |
| License | GPL-3.0 | GPL-3.0 | Apache-2.0 | Proprietary | GPL-3.0 |
| **Cleaning** | | | | | |
| App caches, logs, temp, Trash | Yes | Yes | No | Yes | Yes |
| Browser caches | Yes | Yes | No | Yes | Yes |
| Browser privacy data (cookies, passwords) | No | Yes | No | Yes | No |
| Developer caches (npm, pnpm, uv, pip, Cargo, Go) | Yes | No | No | No | Yes |
| Package manager leftovers | apt, snaps, kernels | apt, snaps | No | N/A | Homebrew |
| Project build folders (`node_modules`, `target`) | Yes | No | No | No | Yes |
| Downloaded installer files | Yes | No | No | No | Yes |
| Dry-run preview before deleting | Yes | Yes | N/A | Yes | Yes |
| Shred files, wipe free space | No | Yes | No | Yes | No |
| **System and processes** | | | | | |
| Uninstall with leftovers | apt, snap, Flatpak, AppImage | No | No | Uninstaller only | Yes |
| Disk usage explorer | Yes | No | No | Yes | Yes |
| Live telemetry (CPU, RAM, Disk, Net) | Yes | No | Yes | No | Yes |
| Temperatures and battery health | Yes | No | Yes | No | Yes |
| System maintenance (DNS, font caches, trim) | Yes | No | No | Registry only | Yes |

Preen does not handle browser cookies, password databases, secure file wiping, startup applications, duplicate files, or GPU statistics. BleachBit remains suitable for file shredding, and btop for process termination.

## Install

Ubuntu or Kubuntu 26.04 (Qt 6.10 and KDE Frameworks 6):

```bash
wget https://github.com/youxufkhan/preen/releases/download/v0.1.1/preen_0.1.1_amd64.deb
sudo apt install ./preen_0.1.1_amd64.deb
```

On other distributions, use the command-line engine on its own (`preen-linux-amd64` on the [releases page](https://github.com/youxufkhan/preen/releases)) or build from source (see Build below).

Then open **Preen** from the app menu, or use the terminal:

```bash
preen clean --dry-run        # see what would be removed
preen clean                  # remove the selected items (asks first)
preen uninstall --list       # installed apps and their ids
preen uninstall apt:btop --dry-run
preen analyze ~              # what uses space in your home folder
preen status --watch         # live dashboard
preen purge --dry-run        # build folders in your projects
preen installer --dry-run    # installer files in Downloads
preen optimize               # maintenance and health checks
preen history                # what Preen removed
```

Every command that removes something accepts `--dry-run`, `--yes`, `--json`, `--all` (also select items marked review) and `--only <id>`.

## Safety

- Every delete goes through one check (`internal/safe`). It refuses system folders, top-level folders, your home folder, `~/.ssh`, `~/.gnupg`, keyrings, browser password and cookie files, and symlinks that lead into any of them. A fuzz test guards it.
- Preen never runs as root. System items go through `pkexec preen helper <action>`, which accepts only a fixed list of named actions and re-checks each one. Removing a package that would take a desktop metapackage or a system package with it is refused.
- Apps that are running are skipped: Chrome, Electron apps, Firefox and Gradle are detected through their lock files or processes.
- Uninstall leftovers and Disk deletions go to the Trash. Clean, Projects and Installers delete permanently, as Mole does.
- Sizes count only space that would really be freed. A cache whose files are hard-linked into projects (uv, pnpm, Bun) is not over-reported.
- Every removal is logged to `~/.local/state/preen/operations.log` (see `preen history`). `PREEN_NO_OPLOG=1` turns the log off.
- Never clean something: add its path, or an item id such as `apt-cache`, to `~/.config/preen/whitelist`. If that file can't be read, Preen stops.
- Choose project folders: list them, one per line, in `~/.config/preen/purge_paths`.

## Build

```bash
sudo apt install golang-go g++ qt6-base-dev qt6-declarative-dev
make            # build/preen (engine) and build/preen-gui (Kirigami GUI)
make test       # Go tests, including the delete-safety fuzz seeds
make deb        # build/preen_<version>_amd64.deb
```

Layout:

- `*.go`: the engine, using only the Go standard library. There is one file per command, plus `engine.go` (events, sizes, plan runner) and `helper.go` (root actions).
- `internal/safe`: the delete validator.
- `gui/`: about 100 lines of C++ that run `preen <cmd> --json` and pass each JSON line to QML, plus the Kirigami pages in `gui/qml`.
- `packaging/`: the desktop file, polkit policy and Debian control file.

`PREEN_PAGE=clean PREEN_SCREENSHOT=out.png QT_QPA_PLATFORM=offscreen build/preen-gui` renders a page to a PNG without opening a window.

## License

GPL-3.0. Preen is an independent project and is not affiliated with Mole.
