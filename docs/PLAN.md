# План разработки V2Scale

Цель: клиент на базе V2RayNG, где tailnet (в том числе Headscale) и MagicDNS работают внутри
того же VPN и процесса. Каждая фаза заканчивается проверяемым результатом (критерий
готовности). Фазы упорядочены так, чтобы самые рискованные неизвестные закрывались как можно
раньше. Отметка `[ ]` - не сделано, `[x]` - сделано (обновляем по ходу, детали в DEVLOG.md).

Принципы:
- Сначала воспроизводимая сборка, потом изменения. Ни одной правки, пока upstream не собран и
  не запущен на вашем телефоне.
- Каждая гипотеза из [RESEARCH.md](RESEARCH.md) проверяется отдельным маленьким экспериментом,
  результат записывается в DEVLOG.md.
- Основная логика (Go) разрабатывается и тестируется на Mac без Android, на устройство
  переносится готовым.
- Изменения в upstream-коде V2RayNG делаем небольшими коммитами, чтобы `git merge upstream/master`
  оставался возможным.
- Всё, что устанавливается на вашу машину, записывается в [INSTALLED-TOOLS.md](INSTALLED-TOOLS.md).

---

## Фаза 0. Воспроизводимая сборка upstream

Цель: собрать неизменённый V2RayNG 2.3.12 из исходников на этом Mac и запустить на Nothing Phone (3).

- [x] 0.1 Установить недостающее: JDK 21, Android SDK `platforms;android-37.0`,
  `build-tools;37.0.0`, NDK `30.0.16248370` (SDK-инструменты командной строки и `gomobile` уже
  есть). Записать в INSTALLED-TOOLS.md.
- [x] 0.2 Собрать `libhev*` (`compile-hevtun.sh`, нужен `NDK_HOME`) только для `arm64-v8a`
  (для разработки хватает одной архитектуры; полный набор ABI добавим в Фазе 7).
- [x] 0.3 Собрать `libv2ray.aar` из `libv2scale/` (`gen_assets.sh` скачивает geo-данные,
  `gomobile bind -androidapi 24 -trimpath -ldflags='-s -w -buildid= -checklinkname=0'`),
  положить в `V2rayNG/app/libs/`.
- [x] 0.4 Собрать APK (`./gradlew assembleFdroidDebug` или `assemblePlaystoreDebug`, выбрать
  вариант без Google-сервисов), подписать debug-ключом.
- [x] 0.5 Заменить `applicationId`/название (временно `org.v2scale.android`, "V2Scale"), чтобы
  ставить рядом с V2RayNG. Проверить, что `applicationIdSuffix` flavors не ломает проект.
- [x] 0.6 Установить на телефон, импортировать профиль (подписку 3x-ui), включить VPN, убедиться
  в обычной работе.
- [x] 0.7 Скрипты `scripts/build-core.sh`, `scripts/build-app.sh` для повторяемой сборки одной
  командой; описать в DEVLOG.md.

Критерий: `scripts/build-app.sh` из чистого клона даёт APK, который ведёт себя как V2RayNG.

Изучить: `.github/workflows/build.yml`, `V2rayNG/app/build.gradle.kts`, `compile-hevtun.sh`,
`libv2scale/README` (инструкция сборки).

---

## Фаза 1. Go-эксперименты на Mac: Xray и tsnet в одном процессе

Цель: доказать, что outbound `tailnet` в Xray работает, без Android.

- [x] 1.1 Стенд (прогон против реального Headscale отложен до появления тестового ключа): Go-модуль `libv2scale` собирается на Mac. Тестовый control-сервер: пакет
  Tailscale `tstest/integration/testcontrol` (герметично, без интернета), плюс
  опциональный прогон против вашего реального Headscale (понадобится тестовый pre-auth ключ).
- [x] 1.2 Выяснить: принимает ли `core.New` конфигурацию, где правило роутинга ссылается на
  outbound-тег, добавляемый позже через `Manager.AddHandler`; выбрать между «плейсхолдер
  `freedom` с тем же тегом + `RemoveHandler`/`AddHandler`» и «добавление до `Start`».
- [x] 1.3 Каркас пакета `libv2scale/tailnet`: `Handler` (интерфейс `outbound.Handler`),
  `Config` (JSON), запуск `tsnet.Server` внутри. Только TCP, домены и IP.
- [x] 1.4 Тест-стенд: два tsnet-узла (клиент в Xray, «сервер» с эхо-слушателем), SOCKS-inbound
  Xray и роутинг `100.64.0.0/10 -> tailnet`. Проверить проход TCP.
- [x] 1.5 Таблица имён для `extra_records` Headscale (перенести из `tsnet-forward`).
- [x] 1.6 Юнит-тесты и бенчмарк на пропускную способность/память/горутины, утечки на Stop/Start.

Критерий: `go test ./...` в `libv2scale` проходит; curl через SOCKS-inbound Xray достаёт сервис
в тестовом tailnet, а «обычный» трафик по-прежнему идёт через proxy/direct.

Изучить (Xray-core): `core/` (`New`, `Instance`, функции), `app/dispatcher`,
`app/router` (как разрешается `outboundTag`), `app/proxyman/outbound` (менеджер, `Handler`),
`common/session`, `transport.Link`, `proxy/freedom`, `common/buf`, `common/task`.
Изучить (Tailscale): `tsnet/tsnet.go` (старт, `Dir`, `Store`, `Logf`), `net/tsdial`,
`wgengine/netstack`, `tstest/integration/testcontrol`.

---

## Фаза 2. UDP, DNS-запросы и полнота outbound

- [x] 2.1 UDP-поддержка `tailnet` outbound (DNS на `100.100.100.100:53`, QUIC к пирам):
  `tsnet.Server.ListenPacket`/обёртка с таблицей соответствий потоков, таймауты.
- [x] 2.2 Передача DNS-запросов в MagicDNS: Xray DNS-сервер `100.100.100.100`, роутинг его
  трафика в `tailnet`, ответы даёт DNS-мост в обработчике (см. RESEARCH.md: сетевой путь через netstack
  пересылает запросы наружу). Тест: резолв имён пиров и `extra_records`.
- [x] 2.3 Статистика трафика outbound (интерфейс `stats`, чтобы V2RayNG показывал скорость).
- [x] 2.4 Ошибки и состояние: статусы «подключается / ждёт входа / работает», URL интерактивной
  авторизации (когда нет auth key), callback в Kotlin.
- [x] 2.5 Конфигурация: `SetTailnetConfig(json)` в `CoreController`, валидация, примеры.

Критерий: имена из tailnet резолвятся и открываются через Xray-inbound на Mac без Android.

Изучить: `proxy/dns`, `app/dns` (серверы, `domains`/`expectIPs`), `app/dns/fakedns`,
`net/dns/resolver` и обработчик `handleMagicDNSUDP` в Tailscale, `features/stats`.

---

## Фаза 3. Android: встраивание ядра

- [ ] 3.1 Хуки Android: `netmon.RegisterInterfaceGetter` (интерфейсы из Kotlin JSON-ом),
  обновление маршрута по умолчанию и `InjectEvent` при смене сети
  (`service/NetworkMonitor.kt`), каталоги HOME/TMPDIR, `TS_NO_LOGS_NO_SUPPORT`.
  Образец: `tsnet-forward/forwarder/android.go`, `tailscale-android/libtailscale/net.go`.
- [ ] 3.2 Решить, нужен ли `SetAndroidProtectFunc`/`BindToNetwork` (V2RayNG исключает себя из
  VPN, поэтому, вероятно, достаточно BindToNetwork для корректной смены сети): проверить опытом.
- [x] 3.3 Сборка `libv2ray.aar` с tailnet (25,4 МБ, APK 59 МБ); память проверим на устройстве в 3.5.
- [x] 3.4 Kotlin (глобальная страница «Tailnet» в Настройках; статус через MMKV, ключ стирается после входа): настройки tailnet (control URL, auth key, hostname, прокси control-канала),
  передача в ядро перед `StartLoop`, отображение статуса.
- [~] 3.5 Роутинг (правила и DNS-сервер генерируются в `CoreConfigManager`, юнит-тесты есть; проверка на устройстве ждёт ключа): правила `-> tailnet` для доменов и подсетей; ручные тесты на устройстве.
- [ ] 3.6 Смена сети (Wi-Fi <-> LTE, режим полёта, сон): узел должен восстанавливаться без
  перезапуска сервиса (известный класс багов sing-box#4515).
- [ ] 3.7 Автоперезапуск после сбоев, Doze, фоновые ограничения Nothing OS (foreground service
  уже есть в V2RayNG).

Критерий: на телефоне при работающем V2Scale трафик к IP tailnet идёт через `tailnet`
outbound, остальной - как в V2RayNG; переключение сетей не ломает связь.

Изучить: `service/CoreVpnService.kt`, `service/NetworkMonitor.kt`, `core/CoreServiceManager.kt`,
Android `VpnService.Builder`, `ConnectivityManager.NetworkCallback`, `tailscale-android`
(`backend.go`, `net.go`, `callbacks.go`, `interfaces.go`), `net/netmon` Tailscale.

---

## Фаза 4. DNS: сквозной путь и MagicDNS для обычных приложений

Это главное преимущество перед отдельным клиентом: приложения резолвят имена tailnet обычным
системным способом.

- [ ] 4.0 Инструмент `tools/dnsprobe` (Go, GOOS=android, запуск через `adb shell`): UDP/TCP/DoT-запросы
  с замером времени и разбором ответов. `toybox nc -u` на устройстве ненадёжен (зависает даже на
  loopback), поэтому выводы о UDP-DNS по нему не делаем.
- [ ] 4.1 Диагностика: воспроизвести на upstream 2.3.12, доходит ли DNS до `dns-out`, что
  делает Private DNS «Автоматически» с VPN-DNS `1.1.1.1` (наблюдалось в 2.2.6: DoT мимо ядра,
  блокировка 853 ломала DNS).
- [ ] 4.2 Выбрать схему: (а) VPN выдаёт системе внутренний DNS-адрес, обслуживаемый ядром, и
  порт 53 уходит в `dns-out`, DoT на этот адрес недоступен; (б) блокировка DoT плюс рабочий UDP;
  (в) режим «MagicDNS only для tailnet-доменов» через fakedns.
- [ ] 4.3 Реализация и тесты на устройстве: `adb shell ping имя`, браузер, нативные клиенты
  (Calino как контрольное приложение, которое использует системный DNS).
- [ ] 4.4 Поведение при выключенном V2Scale (системный DNS не должен ломаться), при Private DNS
  «Имя хоста провайдера» (строгий DoT): понять и описать ограничения.
- [ ] 4.5 Режим безопасности DNS: все запросы шифруются (DoH внутри ядра), утечки в открытом
  виде исключены; описать угрозы.

Критерий: `https://nextcloud.ezhkin-drive.ezhkinkot.net` (настоящее имя, порт 443) открывается
в браузере и в Calino на телефоне без костылей и без `.localhost`.

Изучить: `core/CoreConfigManager.kt` (DNS и роутинг), `proxy/tun` + README, `app/dns`,
Android Private DNS (режимы, проверка DoT), `net/dns` Tailscale.

---

## Фаза 5. Пользовательский интерфейс и сценарии

- [ ] 5.1 Решить форму UI вместе с вами: отдельный тип записи «Tailnet» в списке профилей
  или глобальная страница настроек (открытый вопрос, см. DECISIONS.md).
- [ ] 5.2 Экран настроек tailnet (Compose, в стиле приложения): control URL, auth key,
  hostname, прокси control-канала (например, i2pd), домены/подсети для tailnet, MagicDNS.
- [ ] 5.3 Интерактивный вход: показ ссылки авторизации, статус узла (IP, пиры), кнопка
  «Выйти / сбросить узел».
- [ ] 5.4 Импорт/экспорт настроек вместе с остальными данными V2RayNG (backup).
- [ ] 5.5 Локализация (en/ru, остальные - по возможности), иконка и название V2Scale.
- [ ] 5.6 Диагностика для пользователя: «tailnet ping», статус DERP/direct, журнал.

Критерий: человек без нашей помощи подключает свой Headscale и открывает сервис по имени.

Изучить: `ui/` (Compose-компоненты, `SettingsActivity`/страницы), `dto/entities/ProfileItem.kt`,
`enums/EConfigType.kt`, `fmt/` (как добавляются типы профилей), `handler/MmkvManager.kt`.

---

## Фаза 6. Безопасность и надёжность

- [ ] 6.1 Хранение секретов: auth key и состояние узла (приватный ключ WireGuard) в приватном
  каталоге; рассмотреть шифрование ключом Android Keystore.
- [ ] 6.2 Модель угроз: другие приложения на устройстве, root, бэкапы (`allowBackup`), логи.
  Убедиться, что в логи не попадают секреты и URL.
- [ ] 6.3 Нет локальных слушающих портов tailnet-трафика; проверка `ss -ltn` на устройстве.
- [ ] 6.4 Закрепление версий Xray-core/Tailscale, процедура обновления с регрессионными
  тестами (`go test`, смоук на устройстве).
- [ ] 6.5 Отключение телеметрии Tailscale (log upload, check for updates, `NetCheck`), проверка
  сетевых соединений приложения (`tcpdump`/Wireshark через точку доступа или `adb`).
- [ ] 6.6 Сравнение с upstream: убедиться, что ни одна защитная настройка V2RayNG не ослаблена.

Критерий: чек-лист угроз пройден, документ `docs/SECURITY.md` описывает модель и ограничения.

---

## Фаза 7. Релиз и сопровождение

- [ ] 7.1 Сборка всех ABI (arm64-v8a, armeabi-v7a, x86_64) и release-вариант, свой ключ подписи
  (хранится вне репозитория).
- [ ] 7.2 CI на GitHub Actions по образцу `.github/workflows/build.yml`.
- [ ] 7.3 Регулярная синхронизация с upstream: `git fetch upstream && git merge upstream/master`,
  разрешение конфликтов в небольших зонах наших изменений.
- [ ] 7.4 Релизные заметки, инструкция пользователя, список ограничений.

---

## Что изучить (сводка)

| Область | Что читать | Зачем |
| ------- | ---------- | ----- |
| V2RayNG | `core/CoreConfigManager.kt`, `CoreServiceManager.kt`, `service/CoreVpnService.kt`, `NetworkMonitor.kt`, `handler/SettingsManager.kt`, `ui/` | Как собирается конфиг, как стартует ядро, где добавлять настройки и роутинг |
| Xray-core | `core/`, `app/router`, `app/dispatcher`, `app/proxyman/outbound`, `proxy/freedom`, `proxy/tun`, `proxy/dns`, `app/dns` | Написать outbound `tailnet`, понять DNS и tun |
| Tailscale | `tsnet`, `net/tsdial`, `wgengine/netstack`, `net/netmon`, `net/netns`, `net/dns`, `ipn/ipnlocal` | Встроить узел, MagicDNS, смена сети |
| tailscale-android | `libtailscale/backend.go`, `net.go`, `callbacks.go`, `interfaces.go` | Эталонные Android-хуки |
| Android | `VpnService`, `ConnectivityManager`, Private DNS, foreground services, Doze | Корректный жизненный цикл и DNS |
| Сеть | WireGuard, DERP, NAT traversal Tailscale (disco), Noise-протокол control-канала | Диагностика связности |
| Headscale | `extra_records`, DERP-карта, ACL (формат политики) | Совместимость и тесты |

## Риски

| Риск | Смягчение |
| ---- | --------- |
| DNS на Android не удаётся сделать надёжным | Фаза 4 вынесена отдельно; fallback - работа по IP/доменам через fakedns и документированные ограничения |
| tsnet внутри процесса Xray потребляет много памяти | Измерения в Фазе 3, ограничение горутин/буферов, при необходимости отдельный процесс (`:tailnet`) с IPC |
| Смена сети ломает узел | Хуки netmon из Фазы 3.1, автоперезапуск узла, тесты на устройстве |
| Конфликты при слиянии с upstream | Малые коммиты, наши изменения в отдельных файлах, регулярные merge |
| Изменения API Xray/Tailscale при обновлении | Закрепление версий, тесты outbound, процедура обновления |

## Открытые вопросы к вам

См. [DECISIONS.md](DECISIONS.md), раздел «Открытые вопросы».
