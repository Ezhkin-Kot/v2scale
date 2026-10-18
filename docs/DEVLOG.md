# Журнал разработки

Записи в обратном хронологическом порядке (новые сверху). Формат: что сделали, что выяснили,
что дальше.

## 2026-10-18 - Фаза 0: сборка upstream

- Решение по объёму v1 зафиксировано (без exit node и subnet routing).
- Установлено: JDK 21, SDK platform 37.0, build-tools 37.0.0, NDK 30.0.16248370
  (см. INSTALLED-TOOLS.md).
- Скрипты в `scripts/`: `env.sh` (пути тулчейна), `build-hevtun.sh` (нативные `libhev*` только
  arm64-v8a, upstream-скрипт запускается без правок через `sed` по переменной `ABIS`),
  `build-core.sh` (`libv2ray.aar`: `gen_assets.sh download` скачивает геоданные в `data/`, их
  надо скопировать в `assets/`, затем `gomobile bind -target=android/arm64`),
  `build-app.sh` (Gradle).
- Нюансы: `gen_assets.sh` в вендоренной копии без бита исполнения - запускаем через `bash`.
  `compile-hevtun.sh` кладёт результат в `libs/` корня репозитория (добавлено в `.gitignore`),
  а скрипт-обёртка копирует его в `V2rayNG/app/libs/`.
- Собрано: `libhev-socks5-tunnel.so`, `libhevsockstun.so` (arm64), `libv2ray.aar` (20 МБ,
  arm64, 44 с сборки ядра).

- Gradle: `scripts/build-app.sh` собирает `assemblePlaystoreDebug` для arm64-v8a: первая сборка
  4 мин 10 с (скачивание зависимостей и Gradle 9.8.1), повторная ~20 с. Предупреждение
  «Unable to strip libraries» безобидно (APK чуть больше; NDK можно прописать в `ndkVersion`,
  как делает CI upstream, когда понадобится).
- Переименование для установки рядом с V2RayNG (три маленьких коммита): `applicationId =
  org.v2scale.android` (`namespace` остался `com.v2ray.ang`, чтобы не трогать пакеты Kotlin);
  `Utils.isXray()` обобщена (раньше проверяла префикс `com.v2ray.ang` и при другом id
  переключила бы ядро на ветку v2fly); `shortcuts.xml` указывает на новый пакет;
  `app_name` заменён на «V2Scale» во всех 10 локализациях (иначе локаль перекроет значение).
  Проверено `aapt2 dump badging`: пакет `org.v2scale.android`, метка V2Scale, arm64-v8a.
- Осталось по Фазе 0: пункт 0.6 - установка на телефон и проверка работы (нужна ваша помощь:
  подтвердить отладку по USB и импортировать профиль).

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
