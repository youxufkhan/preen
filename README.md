# Preen

Clean, uninstall, analyze and monitor Linux from one app. Preen is built for Ubuntu and Kubuntu and inspired by [Mole](https://github.com/tw93/mole) for macOS.

- **Clean**: removes caches, logs, old snap versions, crash dumps, old kernels, browser and Electron app caches, and developer caches (uv, pnpm, npm, Bun, pip, Gradle, Cargo, Go).
- **Uninstall**: removes apt, snap, Flatpak and AppImage apps together with their leftovers. Settings stay unless you select them.
- **Disk**: shows what uses space, folder by folder, and moves things to the Trash.
- **Projects**: removes rebuildable `node_modules`, `target`, `build`, `.venv` and similar folders.
- **Installers**: removes `.deb`, `.rpm`, `.run`, `.exe` and similar files from Downloads and Desktop.
- **Maintenance**: rebuilds caches, flushes DNS, sets snap retention and checks system health.
- **Overview**: live CPU, memory, disk, network, battery, sensors and the busiest processes.

Every change is listed for review first. Nothing runs in the background.

## Install

Ubuntu or Kubuntu 26.04 (Qt 6.10 and KDE Frameworks 6):

```bash
wget https://github.com/youxufkhan/preen/releases/download/v0.1.0/preen_0.1.0_amd64.deb
sudo apt install ./preen_0.1.0_amd64.deb
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
