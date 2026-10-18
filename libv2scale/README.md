# libv2scale

Go-обёртка ядра Xray для V2Scale (gomobile -> `libv2ray.aar`).

Вендорена из [2dust/AndroidLibXrayLite](https://github.com/2dust/AndroidLibXrayLite)
(коммит `1be1e62`, Xray-core v26.10.17), лицензия LGPL-3.0 (см. `LICENSE`).
Go-пакет называется `libv2ray`, чтобы Kotlin-код V2RayNG (`import libv2ray.*`) не менялся.

Изменения относительно upstream обёртки фиксируются в `docs/DEVLOG.md` проекта; сюда будет
добавлен пакет `tailnet/` (outbound Tailscale).

## Сборка

```bash
./gen_assets.sh                       # скачивает geoip/geosite в assets/ (нужны jq, curl)
gomobile init
gomobile bind -v -androidapi 24 -trimpath \
  -ldflags='-s -w -buildid= -checklinkname=0' ./
cp libv2ray.aar ../V2rayNG/app/libs/
```

Результат (`libv2ray.aar`, `libv2ray-sources.jar`) и `assets/*.dat` в git не попадают.
