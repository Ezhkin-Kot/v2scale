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
| (пока ничего) | | | |

## Файлы и каталоги проекта вне репозитория

- `~/Projects/misc/v2scale-ref/` - справочные клоны (`tailscale`, `tailscale-android`,
  `Xray-core`, `v2rayNG`): `rm -rf ~/Projects/misc/v2scale-ref`.
