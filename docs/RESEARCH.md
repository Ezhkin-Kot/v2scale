# Результаты изучения

Пометки достоверности: **[код]** прочитано в исходниках, **[опыт]** наблюдалось на реальном
устройстве (Nothing Phone 3, Android 17), **[док]** взято из документации или issue (не
проверено нами), **[гипотеза]** предположение, которое надо проверить.

Справочные клоны лежат вне репозитория: `~/Projects/misc/v2scale-ref/` (`tailscale`,
`tailscale-android`, `Xray-core`, `v2rayNG`, неглубокие клоны на 2026-10-18). Версии на момент
изучения: V2RayNG 2.3.12 (`118cf5b`), AndroidLibXrayLite `1be1e62` (Xray-core v26.10.17),
Tailscale `e3c3ef1` (2026-10-16, у нас в модулях v1.104.1), Xray-core `92ccc93` (v26.10.17).

## 1. V2RayNG (`V2rayNG/app/src/main/java/com/v2ray/ang/`)

- **Структура [код].** ~186 Kotlin-файлов: `core/` (сборка и запуск конфигурации Xray),
  `service/` (VPN, tun, мониторинг сети), `handler/` (настройки, подписки, MMKV), `fmt/`
  (парсеры ссылок протоколов), `ui/` (на Compose), `dto/`. Крупнейшие файлы:
  `core/CoreConfigManager.kt` (1317 строк), `core/CoreOutboundBuilder.kt` (708),
  `handler/SettingsManager.kt` (582), `core/CoreServiceManager.kt` (521).
- **Ядро [код].** Подключено как `libv2ray.aar` (gomobile) из AndroidLibXrayLite. Kotlin
  вызывает `libv2ray.Libv2ray`/`CoreController` через `core/CoreNativeManager.kt`
  (`initCoreEnv`, `newCoreController`, `measureOutboundDelay`). В APK ядро - `libgojni.so`
  (~35 МБ), рядом `libhev-socks5-tunnel.so`.
- **Tun [код].** `service/CoreVpnService.kt` строит `VpnService.Builder`: адрес, маршруты
  (`0.0.0.0/0` или список без LAN при `bypassLan`), DNS-серверы из настройки VPN DNS (по
  умолчанию `1.1.1.1`, `AppConfig.DNS_VPN`), per-app proxy. Есть два режима tun: встроенный
  `tun`-inbound Xray (ядру передаётся fd через переменную `xray.tun.fd`, см.
  `libv2ray_main.go`) и `hev-socks5-tunnel` (`TProxyService.kt`).
- **Исключение самого приложения из VPN [код].** `configurePerAppProxy` всегда добавляет
  `addDisallowedApplication(selfPackageName)` (кроме режима «только выбранные», где себя
  исключают из списка). Поэтому все сокеты процесса V2RayNG, включая исходящие соединения
  Xray, идут мимо туннеля, и явный `protect` не нужен. Это же верно для встроенного узла
  Tailscale: его WireGuard/DERP/control трафик не зациклится в tun.
- **DNS [код].** `core/CoreConfigManager.kt`: при включённом Local DNS в VPN-режиме добавляется
  правило роутинга `inboundTag tun (или socks для hev), port 53 -> outbound dns-out` и
  `dns`-outbound. Fake DNS добавляет сервер `fakedns` первым в списке и сниффинг
  `destOverride: fakedns`. Пользовательские DNS hosts кладутся в `dns.hosts`. В файле есть
  захардкоженные hosts для DoT/DoH-серверов популярных провайдеров («fix localhost DNS
  problem» для Android Private DNS).
- **DNS на устройстве [опыт, версия 2.2.6].** Системные запросы шли не в DNS-модуль Xray, а
  DoT-соединением на `1.1.1.1:853` (Android Private DNS в режиме «Автоматически», сеть VPN
  показывала `UsePrivateDns: true`); поэтому «DNS hosts» не применялись. Блокировка порта 853
  в роутинге приводила к потере всего DNS: обычные UDP-запросы на `1.1.1.1:53` не получали
  ответа (в том числе наш тест через `nc -u`). Причина не установлена; возможно, исправлена
  в 2.3.x (в `CoreVpnService.kt` закомментирован прежний путь `addDnsServer(PRIVATE_VLAN4_ROUTER)`).
- **Сборка [код, CI `.github/workflows/build.yml`].** Gradle 9.8.1, AGP 9.4.1, Kotlin 2.4.21,
  `compileSdk 37` (platforms;android-37.0, build-tools;37.0.0), NDK 30.0.16248370, JDK 21 в CI.
  `libv2ray.aar` кладётся в `V2rayNG/app/libs/`, нативные `libhev*` собирает
  `compile-hevtun.sh` через `ndk-build`. Есть flavors `fdroid`/`playstore`,
  `applicationId = com.v2ray.ang`.

## 2. Обёртка ядра (бывшая AndroidLibXrayLite, теперь `libv2scale/`)

- **Размер [код].** ~580 строк Go, зависимости: `github.com/xtls/xray-core`,
  `github.com/apernet/quic-go`, `golang.org/x/mobile`.
- **Старт ядра [код].** `libv2ray_main.go`: `CoreController.StartLoop(config, tunFd)`
  выставляет `xray.tun.fd`, парсит JSON `coreserial.LoadJSONConfig`, `core.New(config)`,
  `Start()`. Импорт `_ ".../main/distro/all"` подключает все стандартные прокси.
  `core.Instance` доступен как `x.coreInstance`, значит можно вызвать
  `GetFeature(outbound.ManagerType())` и `AddHandler` для нашего outbound.
- **Сборка [док из README].** `gomobile init`, затем
  `gomobile bind -v -androidapi 24 -trimpath -ldflags='-s -w -buildid= -checklinkname=0' ./`
  (флаг `-checklinkname=0` нужен Xray). Геоданные (`assets/*.dat`, ~26 МБ) скачивает
  `gen_assets.sh`, они в `.gitignore`.
- **ProcessFinder [код].** `libv2ray_android.go` регистрирует поиск процесса по соединению
  (per-app роутинг по UID).
- **Лицензия:** LGPL-3.0 (V2RayNG - GPL-3.0).

## 3. Xray-core

- **Расширяемость outbound [код].** `features/outbound/outbound.go`: публичный (`xray:api:stable`)
  интерфейс `Handler` (`common.Runnable` + `Tag`, `Dispatch(ctx, link)`, `SenderSettings`,
  `ProxySettings`) и `Manager` с `AddHandler`/`RemoveHandler`/`GetHandler`. Роутер выбирает
  outbound по тегу в момент диспетчеризации, поэтому правило с `outboundTag: "tailnet"`
  можно загрузить до добавления самого обработчика [гипотеза, проверить на `core.New`].
- **Образец обработчика [код].** `proxy/freedom/freedom.go: (*Handler).Process` берёт цель из
  `session.OutboundsFromContext(ctx)` (последний элемент, `Target`, домен или IP), использует
  `link.Reader/Writer`, повторные попытки (`retry.ExponentialBackoff`), копирование через
  `buf/task`. Начиная с v25.10 для VLESS reverse у `freedom` есть `finalRules` и блокировка
  приватных адресов по умолчанию [код+док].
- **Tun [код].** `proxy/tun/` (~11 тыс. строк, gVisor): TCP, UDP, ICMP echo; Android-вариант
  читает fd из `xray.tun.fd`. README подчёркивает: DNS, который выдаёт ОС, нужно направить в
  tun и в роутинге отправить порт 53 в `dns` outbound (`{"inboundTag":["tun"],"port":53,
  "outboundTag":"dns"}`); upstream DNS-сервера не должны зависеть от системного резолвера
  (иначе петля).
- **DNS [код].** `proxy/dns/dns.go`: `dns`-outbound использует `features/dns.Client` и
  опциональный `FakeDNSEngine`. Серверы DNS-модуля задаются адресами (`udp/tcp/https/quic`,
  `fakedns`, `localhost`); плагинов нет.
- **Нативной поддержки Tailscale нет [док]** (подтверждено тремя независимыми источниками при
  поиске, форков с tsnet не найдено).
- **VLESS reverse [док].** Есть, но это иной механизм (обратный туннель через VPS), не наша
  цель; см. обсуждение в истории проекта.

## 4. Tailscale

- **tsnet [код].** `tsnet/tsnet.go`: `Server{Dir, Hostname, ControlURL, AuthKey, Ephemeral,
  Store, ...}`, `Start`, `Up`, `Dial(ctx, network, addr)`, `ListenPacket`, `LocalClient`,
  `HTTPClient`, `CapturePcap`. Userspace WireGuard и netstack (gVisor), второй TUN не нужен.
- **MagicDNS [код].** `wgengine/netstack/netstack.go`: трафик на сервисный IP `100.100.100.100`
  порт 53 по TCP (`ns.dns.HandleTCPConn`) и UDP (`handleMagicDNSUDP`) обслуживается
  встроенным резолвером. Значит, DNS-запрос можно отправить в tailnet через `tsnet.Dial` на
  `100.100.100.100:53`, и ответит MagicDNS/split DNS tailnet. [гипотеза: ответы для
  `extra_records` Headscale зависят от Type записи, см. ниже].
- **Имена в `Dial` [код, опыт].** `ipn/ipnlocal/node_backend.go`: `tsdial` резолвит имена пиров
  (FQDN и короткие) и `DNS.ExtraRecords` **только с пустым Type**. Headscale отдаёт
  `extra_records` с `type: "A"`, и они игнорируются: `Dial("name:443")` даёт «no such host»
  (проверено на нашем Headscale). Обход, реализованный в `tsnet-forward`: брать
  `lc.DNSConfig(ctx).ExtraRecords` (`client/local`) и подставлять IP до вызова `Dial`.
- **Android-хуки [код].** `net/netns/netns_android.go`: `SetAndroidProtectFunc(func(fd) error)`
  и `SetAndroidBindToNetworkFunc`; `net/netmon/interfaces_android.go`:
  `UpdateLastKnownDefaultRouteInterface`, `UpdateLastKnownDefaultGateway`, а в
  `netmon.RegisterInterfaceGetter` Java отдаёт список интерфейсов (netlink запрещён
  приложениям с Android 11, `net.Interfaces` падает). Официальное приложение
  (`tailscale-android/libtailscale/{backend,net,tailscale,callbacks}.go`) делает ровно это.
  В gomobile-сборках (cgo) встроенный fallback `feature/androidbin` не регистрируется.
  Готовая реализация нашего getter - `tsnet-forward/forwarder/android.go`.
- **Паника без каталога логов [опыт].** `logpolicy` паникует «no safe place found to store log
  state», если нет HOME/TMPDIR; нужно задать каталоги в приватном хранилище и
  `TS_NO_LOGS_NO_SUPPORT=true` (так же отключается отправка логов Tailscale).
- **Заготовки из `tsnet-forward` [код, опыт]:** ограниченный локальный CA, таблица имён,
  валидация конфигурации, тесты на Go; работает на Android 17.

## 5. sing-box как альтернатива (изучено, не выбрано)

- Эндпоинт `tailscale` с `control_url` и DNS-сервер `tailscale` есть с версии 1.12 [док].
- Известные проблемы на Android в режиме VPN [док]: SagerNet/sing-box#3755 (устройство не
  достаёт до пиров, закрыт как «not planned»), #4515 (эндпоинт не восстанавливается после
  смены сети, исправление только в форках). Эти же классы багов надо учесть и нам.

## 6. Неизвестное, что предстоит выяснить

| Вопрос | Ответ / как проверить | Фаза |
| ------ | --------------------- | ---- |
| Работает ли DNS-перехват порта 53 в V2RayNG 2.3.x | Не выяснено; нужен `dnsprobe` (в 2.2.6 не работал) | 4 |
| Принимает ли `core.New` правило на несуществующий outbound-тег | **Да.** `core.New` грузит конфиг, обработчик можно добавить через `AddHandler` и до, и после `Start` (тесты `tailnet/instance_test.go`) | ~~1~~ |
| Что будет, если обработчика с тегом нет | **Соединение падает** (fail closed), не уходит в другой outbound | ~~1~~ |
| Конфликт тега с готовым outbound | `AddHandler` возвращает `existing tag found`; `RemoveHandler` + `AddHandler` работает | ~~1~~ |
| Приходят ли домены в обработчик неразрешёнными | **Да**, `Target.Address` - домен; tsnet резолвит MagicDNS сам | ~~1~~ |
| Совместимы ли зависимости Xray и Tailscale в одном модуле | **Да.** Go выбрал более новые gVisor и wireguard-go от Tailscale; тесты Xray `proxy/{tun,wireguard,dns}` проходят; ядро с этими версиями работает на телефоне | ~~1~~ |
| Работает ли `Dial` на `extra_records` Headscale с типом A | **Нет** (подтверждено герметичным тестом); обход - `tailnet/names.go` | ~~1~~ |
| Скорость, память, утечки tsnet в процессе Xray (на Mac) | 16 МиБ в обе стороны за 291 мс (~55 МиБ/с), куча ~32 МиБ, 40 параллельных соединений; 4 цикла старт/стоп без роста горутин (6 -> 6) | ~~1~~ |
| Нужен ли UDP в `tailnet` outbound | Да (DNS на `100.100.100.100:53`, QUIC); не реализован | 2 |
| Достаточно ли `100.100.100.100:53` для `extra_records` Headscale | Проверить на нашем Headscale | 4 |
| Как ведёт себя tsnet при смене сети Wi-Fi/мобильная внутри VPN | Ручные тесты на устройстве | 3 |
| Размер APK и память на телефоне с tsnet | Сборка и измерения | 3 |
