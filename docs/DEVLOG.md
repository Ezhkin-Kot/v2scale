# Журнал разработки

Записи в обратном хронологическом порядке (новые сверху). Формат: что сделали, что выяснили,
что дальше.

## 2026-10-18 - GitHub-fork и ветки

- Репозиторий на GitHub пересоздан как настоящий fork `2dust/v2rayNG` (`Ezhkin-Kot/v2scale`,
  публичный, ветка по умолчанию `master`). `origin` указывает на него.
- Ветка по умолчанию остаётся `master`; отдельная `main` не нужна. Коммиты: Conventional
  Commits, без соавторов.

## 2026-10-18 - подготовка проекта

- Создан репозиторий: полный клон `2dust/v2rayNG` (HEAD `118cf5b`, 2.3.12, ~1768 коммитов),
  remote переименован в `upstream`, рабочая ветка `main`.
- Подмодуль `AndroidLibXrayLite` удалён, его исходники вендорены в `libv2scale/` (коммит
  `1be1e62`, Xray-core v26.10.17). Подмодуль `hev-socks5-tunnel` оставлен.
- Справочные клоны (неглубокие): `~/Projects/misc/v2scale-ref/{tailscale, tailscale-android,
  Xray-core, v2rayNG}`.
- Изучены (см. RESEARCH.md): структура V2RayNG и сборка, обёртка ядра, outbound-интерфейс
  Xray, DNS-перехват V2RayNG, tun Xray, хуки Tailscale для Android, MagicDNS на
  `100.100.100.100`.
- Составлены PLAN.md, ARCHITECTURE.md, DECISIONS.md, RESEARCH.md.
- Проверено: вендоренный `libv2scale` собирается на Mac (`go build ./...`, 22 с, зависимости Xray-core
  скачались в кэш модулей), `go.mod`/`go.sum` не изменились.
- Следующий шаг: Фаза 0 (воспроизводимая сборка upstream), для неё нужно поставить JDK 21,
  Android SDK platform 37, build-tools 37.0.0, NDK 30.0.16248370 (см. INSTALLED-TOOLS.md).
