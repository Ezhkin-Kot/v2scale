# V2Scale

Форк [V2RayNG](https://github.com/2dust/v2rayNG) (Android-клиент Xray) со встроенным клиентом
Tailscale. Цель: один VPN-клиент, который сохраняет все возможности и модель безопасности
V2RayNG (VLESS/Reality, роутинг, per-app proxy, подписки) и **заодно** держит узел tailnet
внутри того же процесса, с поддержкой self-hosted координационных серверов (Headscale) и
MagicDNS.

Зачем: на Android одновременно активен только один `VpnService`. Сейчас V2RayNG и
официальный Tailscale взаимоисключают друг друга, а дополнительные обходные приложения
(как `tsnet-forward`) требуют отдельного процесса, исключений из VPN и локальных адресов.
В V2Scale tailnet становится ещё одним outbound внутри Xray: правило роутинга
«этот домен или подсеть -> tailnet» работает так же, как «-> proxy» или «-> direct».

**Статус:** подготовка. Репозиторий создан, исходники изучены, план составлен.
Следующий шаг - Фаза 0 (воспроизводимая сборка upstream). См. [docs/PLAN.md](docs/PLAN.md).

## Документация

| Файл | Содержание |
| ---- | ---------- |
| [docs/PLAN.md](docs/PLAN.md) | Пошаговый план по фазам, критерии готовности, что изучить |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Целевая архитектура и потоки данных |
| [docs/RESEARCH.md](docs/RESEARCH.md) | Что выяснено по исходникам V2RayNG, Xray-core и Tailscale (с путями к файлам) |
| [docs/DECISIONS.md](docs/DECISIONS.md) | Принятые решения и открытые вопросы |
| [docs/DEVLOG.md](docs/DEVLOG.md) | Журнал разработки |
| [docs/INSTALLED-TOOLS.md](docs/INSTALLED-TOOLS.md) | Какие инструменты установлены ради проекта и как их удалить |

## Структура репозитория

- Всё из upstream V2RayNG: `V2rayNG/` (приложение), `hev-socks5-tunnel/` (подмодуль), `compile-hevtun.sh`.
- `libv2scale/` - Go-обёртка ядра (вендорена из [AndroidLibXrayLite](https://github.com/2dust/AndroidLibXrayLite),
  LGPL-3.0, исходная точка - коммит `1be1e62`). Сюда добавляется Tailscale. Go-пакет сохраняет имя
  `libv2ray`, чтобы Kotlin-код менялся минимально.
- `docs/` - документация проекта.

Ветки: `main` - наша разработка. `master` - зеркало upstream (`git fetch upstream`).
Подробности синхронизации - в [docs/DECISIONS.md](docs/DECISIONS.md).

## Лицензия

GPL-3.0 (как у V2RayNG). `libv2scale/` - LGPL-3.0 (как AndroidLibXrayLite); Xray-core - MPL-2.0;
Tailscale - BSD-3-Clause.
