# Установленные инструменты

Список всего, что установлено ради V2Scale, с командами удаления. Раздел «Уже было» не
удаляется. Записи добавляются сразу после установки.

## Уже было до начала проекта (не удалять)

- Go 1.27.x, `gomobile` и `gobind` (`~/go/bin`), JDK 17 (`openjdk@17`, brew), `openjdk` 27 (brew),
  `gradle` 9.8.1 (brew), Android command-line tools (`/opt/homebrew/share/android-commandlinetools`)
  с `platforms;android-35`, `build-tools;35.0.0`, `ndk;27.0.12077973`, `adb` (platform-tools).
  Они ставились для проекта `tsnet-forward` (см. `ezhkin-server-docs/tsnet-forward/INSTALLED-TOOLS.md`).

## Установлено ради V2Scale

| Что | Откуда | Где лежит | Удаление |
| --- | ------ | --------- | -------- |
| `openjdk@21` (JDK 21, для Gradle/AGP 9.4.1, как в CI) | brew formula | `/opt/homebrew/opt/openjdk@21` | `brew uninstall openjdk@21` |
| Android SDK Platform 37.0, Build-Tools 37.0.0 | `sdkmanager` | `/opt/homebrew/share/android-commandlinetools/{platforms/android-37.0,build-tools/37.0.0}` | `sdkmanager --uninstall "platforms;android-37.0" "build-tools;37.0.0"` |
| NDK 30.0.16248370 (~2 ГБ, нужен для `libhev*`, версия из CI upstream) | `sdkmanager` | `/opt/homebrew/share/android-commandlinetools/ndk/30.0.16248370` | `sdkmanager --uninstall "ndk;30.0.16248370"` |

Лицензии Android SDK приняты (`sdkmanager --licenses`) при установке. Каталог SDK целиком
(~6.2 ГБ с прежними пакетами) удаляется командой `brew uninstall --cask android-commandlinetools`
и `rm -rf /opt/homebrew/share/android-commandlinetools`.

Побочно появляются и могут быть удалены: `~/.gradle` (кэш Gradle и зависимостей),
`~/.android`, кэш модулей Go `~/go/pkg/mod`.

## Файлы и каталоги проекта вне репозитория

- `~/Projects/misc/v2scale-ref/` - справочные клоны (`tailscale`, `tailscale-android`,
  `Xray-core`, `v2rayNG`): `rm -rf ~/Projects/misc/v2scale-ref`.
