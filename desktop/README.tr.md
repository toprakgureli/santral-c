# Santral-C Windows Masaüstü Mini Uygulaması

[English](README.md) | Türkçe

Santral-C web paneli ile %100 senkronize, tarayıcı pencerelerinden bağımsız olarak Windows masaüstünde çalışan şeffaf, sürüklenebilir ve her zaman en üstte kalabilen mini telefon uygulaması.

---

## Mimari ve Entegrasyon

- **Sıfır Ek Yük ve SIP Oturumu Paylaşımı:** WebRTC ses akışları, mikrofon izinleri ve SIP oturumu tarayıcı panelinde çalışır. Masaüstü uygulaması sıfır sunucu yüküyle hafif bir uzaktan kumanda görevi görür; ikinci bir SIP kaydı gerekmez.
- **Yerel WebSocket Köprüsü (`127.0.0.1:48123`):** Masaüstü mini widget'ı ile aktif panel sekmesini çift yönlü gerçek zamanlı bağlar.
- **Dinamik Origin ve Güvenlik:**
  - Yalnızca `127.0.0.1` (loopback) üzerinden dinler; dış ağlara kapalıdır.
  - HTTP Upgrade aşamasında Origin kontrolü yapar. İzin verilen adresler `widget-config.json` (`panelUrl`), ortam değişkenleri (`SANTRAL_PANEL_ORIGINS` veya `SANTRAL_PANEL_URL`) üzerinden dinamik olarak yapılandırılabilir.
  - Electron içinde güvenli izolasyon (`nodeIntegration: false`, `contextIsolation: true`).

---

## Özellikler

- **Her Zaman Üstte & Çerçevesiz:** Tüm pencerelerin ve tam ekran uygulamaların üzerinde kayar.
- **Konum Hafızası:** Kapatılıp açıldığında en son taşındığı ekran koordinatlarını hatırlar.
- **Dinamik Yükseklik:** Tuş takımı (DTMF) veya aktarma çekmecesi açıldığında pencere boyutu içeriğe göre otomatik uzar/kısalır.
- **Sistem Tepsisi (Tray) & Menü:** Windows bildirim alanına simge durumuna küçülür; göster/gizle, paneli aç ve çıkış seçenekleri sunar.
- **Global Kısayol Tuşları (Windows Genelinde):**
  - <kbd>Ctrl</kbd> + <kbd>Alt</kbd> + <kbd>A</kbd>: Çağrıyı Cevapla
  - <kbd>Ctrl</kbd> + <kbd>Alt</kbd> + <kbd>H</kbd>: Çağrıyı Kapat / Reddet
  - <kbd>Ctrl</kbd> + <kbd>Alt</kbd> + <kbd>M</kbd>: Mikrofon Sustur / Aç
- **Windows Yerel Bildirimi (Toast):** Mini uygulama arka plandayken veya simge durumundayken gelen aramalarda masaüstü bildirimi çıkar.
- **Hızlı Kontroller:** Ön ek seçici (+90 / Dahili), Sustur, Beklet, DTMF tuşları, Aktarma, canlı çağrı sayacı ve tek tıkla numara kopyalama.

---

## Başlarken

### Geliştirme Modunda Çalıştırma
```bash
cd desktop
npm install
npm start
```

### Testleri Çalıştırma
```bash
cd desktop
npm test
```

### .exe Çıktısı Üretme (Paketleme)
Tek tıkla çalışan bağımsız Windows çalıştırılabilir dosyalarını üretmek için:

```bash
cd desktop

# Hem Portable hem de Kurulumlu (Setup) .exe paketlerini desktop/dist/ içine derler:
npm run dist

# Veya isteğe bağlı olarak tek bir hedef için:
npm run dist:portable    # Çıktı: Santral-C-Widget-Portable.exe
npm run dist:installer   # Çıktı: Santral-C Mini Widget Setup 1.0.0.exe
```

### Yapılandırma
İsteğe bağlı olarak panel adresi ortam değişkeni veya `userData` altındaki `widget-config.json` ile belirlenebilir:

```bash
# Ortam değişkeni ile:
set SANTRAL_PANEL_URL=https://santral.ornek.com
npm start
```
