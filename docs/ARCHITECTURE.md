# Архитектура V2Scale (целевая)

Этот документ описывает, **как мы собираемся** построить V2Scale. Части, отмеченные
«гипотеза», ещё не проверены экспериментом и описаны в [PLAN.md](PLAN.md) как задачи
проверки. Факты об исходниках - в [RESEARCH.md](RESEARCH.md).

## Идея

Xray-core расширяем изнутри: у него есть публичный интерфейс `outbound.Handler` и менеджер
`outbound.Manager.AddHandler`. Tailscale можно встроить как библиотеку (`tsnet`) в тот же
Go-процесс. Поэтому tailnet появляется в Xray как **ещё один outbound с тегом `tailnet`**:
правила роутинга Xray направляют в него нужный трафик, а встроенный узел Tailscale
(userspace WireGuard + netstack, без второго `VpnService`) доставляет его до пиров.

Форк самого Xray-core не нужен: V2RayNG использует ядро как Go-модуль через обёртку
(`libv2scale/`, бывшая AndroidLibXrayLite), а `AddHandler` публичный.

## Общая схема

```
 приложения Android
        |  (все пакеты)
        v
 VpnService tun (fd)  ---- приложение V2Scale исключено из VPN (addDisallowedApplication),
        |                  поэтому его собственные сокеты идут мимо туннеля
        v
 Xray-core, inbound "tun"  (userspace-стек, TCP/UDP, hijack DNS)
        |
        v
 routing (правила V2RayNG + правила tailnet)
   |            |             |              |
   v            v             v              v
 proxy        direct       dns-out        tailnet  <-- наш outbound.Handler
 (VLESS+      (freedom)    (DNS-модуль      |
 Reality)                   Xray)           v
   |                          |        tsnet.Server (в этом же процессе)
   |                          |          - Dial TCP/UDP в tailnet
   |                          |          - MagicDNS: запросы на 100.100.100.100:53
   |                          |          - control: Headscale / Tailscale
   v                          v          - WireGuard UDP + DERP наружу (мимо tun)
 интернет через VPS      MagicDNS-имена
```

## Компоненты

### 1. `libv2scale/` (Go, gomobile -> libv2ray.aar)

- Остаётся обёрткой `CoreController` (StartLoop/StopLoop/измерения), как в AndroidLibXrayLite.
- Добавляется пакет `tailnet/`:
  - `Handler` реализует `outbound.Handler` (`Tag`, `Dispatch(ctx, link)`, `Start`, `Close`,
    `SenderSettings`, `ProxySettings`).
  - `Dispatch` читает последнюю запись `session.OutboundsFromContext(ctx)` (`Target`), дозванивается
    через `tsnet.Server.Dial` (TCP и подключённый UDP, один пир на поток, без full-cone) и связывает
    с `link.Reader/Writer` по образцу `freedom`; счётчики трафика регистрируются под стандартными
    именами `outbound>>>tailnet>>>traffic>>>uplink|downlink`.
  - Имена приходят как домены (после fakedns или сниффинга) и резолвятся самим tsnet
    (MagicDNS пиров плюс наша таблица для `extra_records`).
- Конфигурация tailnet передаётся отдельным JSON, не через конфиг Xray:
  `CoreController.SetTailnetConfig(json)` (до `StartLoop`), состояние - `TailnetStatus()` (JSON:
  `state`, `auth_url`, `ips`; Kotlin опрашивает его, колбэка нет). Так V2RayNG-конфиг остаётся
  валидным для обычного Xray, а его JSON-загрузчик нам не мешает.
- Android-хуки Tailscale (см. [RESEARCH.md](RESEARCH.md)): `netmon.RegisterInterfaceGetter`
  (список интерфейсов из Java, netlink запрещён), обновление маршрута по умолчанию и
  `InjectEvent` при смене сети, `netns.SetAndroidBindToNetworkFunc`, при необходимости
  `SetAndroidProtectFunc`.
- Жизненный цикл: узел поднимается вместе с ядром и останавливается вместе с ним; состояние
  (ключ узла) лежит в приватном каталоге приложения.

### 2. Приложение (Kotlin, `V2rayNG/`)

- Новый тип записи «Tailnet» (или глобальная страница настроек, решение откроем в Фазе 5):
  control URL, auth key / интерактивный вход, имя узла, список маршрутов
  (домены и подсети, уходящие в tailnet), MagicDNS вкл/выкл, прокси для control-канала.
- `CoreConfigManager` добавляет в сгенерированный конфиг Xray правила роутинга
  `outboundTag: "tailnet"` для выбранных доменов/подсетей и, при включённом MagicDNS,
  DNS-сервер для tailnet-доменов.
- Идентификатор приложения меняется (чтобы V2Scale ставился рядом с V2RayNG).

### 3. DNS (отдельная критичная часть)

Проблема, найденная на практике: Android шлёт системные DNS-запросы мимо DNS-модуля Xray
(Private DNS: DoT на `1.1.1.1:853`), поэтому ни hosts, ни fakedns, ни MagicDNS не видят
запросы приложений. Нужен сквозной путь:

1. VPN выдаёт системе DNS-адрес, обслуживаемый ядром (а не публичный `1.1.1.1`).
2. Порт 53 этого адреса уходит в `dns-out` (в V2RayNG это уже есть: правило `inboundTag tun,
   port 53 -> dns-out` при включённом Local DNS).
3. DoT (порт 853) к этому адресу невозможен, поэтому Android в режиме «Автоматически»
   откатывается на обычный DNS (гипотеза, проверим на устройстве).
4. DNS-модуль Xray для tailnet-доменов использует сервер `100.100.100.100` (UDP или TCP),
   запросы к которому через роутинг уходят в `tailnet` outbound. **DNS-мост** в обработчике
   отвечает на них сам через локальный API узла (`QueryDNS`: MagicDNS, split DNS, `extra_records`
   Headscale, в том числе с типом `A`), без обращения по сети к `100.100.100.100`: сетевой путь через
   netstack пересылает запросы на системные DNS-серверы (утечка). Имена вне зон tailnet мост
   отклоняет (`REFUSED`), ничего не уходит наружу. Проверено тестами для UDP и TCP.
5. Остальные домены идут как раньше (remote/domestic DNS, fakedns).

Эти шаги проверяются независимо друг от друга (см. фазы 3-4 в PLAN.md): даже без Tailscale
пункты 1-3 полезны и уже закрывают наблюдавшуюся поломку DNS-хостов.

### 4. Безопасность

- Нет локальных слушающих портов для tailnet-трафика (в отличие от `tsnet-forward`).
- Сокеты Tailscale (WireGuard UDP, control, DERP) исходят из исключённого из VPN
  приложения, поэтому не заходят обратно в tun.
- Отправка логов на серверы Tailscale выключена (`TS_NO_LOGS_NO_SUPPORT`).
- Auth key после первой регистрации стирается. Состояние узла хранится в приватном каталоге
  приложения (рассмотрим шифрование ключом Android Keystore, см. Фазу 6).
- Версии Xray-core и Tailscale закреплены в `go.mod`, обновляются сознательно.
- Весь upstream-код V2RayNG и его защитные настройки сохраняются.

## Что мы сознательно **не** делаем

- Не заменяем `VpnService` и не добавляем второй VPN: одна tun-труба, роутинг решает Xray.
- Не форкаем Xray-core и Tailscale. Если понадобится патч, оформляем его отдельным
  `replace`/патчем и описываем в DECISIONS.md.
- Не включаем exit node и subnet routing в первой версии (возможны позже).
