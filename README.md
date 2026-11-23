# Medatheke by Maximilian

Du wolltest immer alle deine Medien Sortiert haben hier Bitte ein Praktische Mediathek mit vielen Funktionen :D

Medatheke by Maximilian ist eine persönliche Sammlung für Filme, Serien, Anime, Bücher und Manga. Die Windows-EXE startet in einem eigenen Windows-Fenster mit eingebetteter WebView2-Oberfläche. Falls die Laufzeit fehlt, installiert das Setup sie automatisch mit Internetverbindung. Die Sammlung liegt lokal unter `%APPDATA%\Medienregal\sammlung.json`. Für die Datensicherung kann diese Datei kopiert werden. Die Deinstallation über Windows behält die Sammlung.

## Suche und Streaming

Filme und Serien werden in Cinemeta gesucht, Bücher und Manga in der Deutschen Nationalbibliothek und Open Library. Bei Büchern kannst du Titel und Autor, Verlag und Reihe mit Bandnummer oder eine ISBN verwenden. Bei fehlenden Metadaten kannst du den Titel und die Gesamtzahl selbst eintragen oder korrigieren.

Für automatische Streamingangebote in „Möchte ich sehen“ trage unter **Einstellungen** einen Watchmode-API-Schlüssel für dein Land ein. Die App zeigt Abo, kostenlose Angebote, Leihen und Kaufen samt gemeldeten Preisen. Das vorgewählte Land ist Deutschland. Ein TMDB-Read-Access-Token ist optional und ergänzt Titel und Streaminganbieter. TMDB liefert hier keine Einzelpreise. Ohne Schlüssel verlinkt die App auf JustWatch. Anbieterangaben können sich ändern und bei Serien nur Teile der Serie abdecken.

This product uses the TMDB API but is not endorsed or certified by TMDB. Streamingdaten stammen von Watchmode und JustWatch (über TMDB). Katalogdaten stammen auch von Cinemeta, der Deutschen Nationalbibliothek und Open Library.

## Windows-Installer und Veröffentlichungen

Voraussetzungen für den lokalen Build unter Linux sind Go ab 1.23, Bash, cURL und NSIS.

```sh
bash build.sh TheGrifer1337/Medienregal
```

Der Standardordner ist `%LOCALAPPDATA%\Programs\Medienregal`. Im Installer kann ein anderer beschreibbarer Ordner gewählt werden. Die Installation erfolgt pro Windows-Benutzer. Die EXE und der Installer sind für Windows x64 gebaut.

Für GitHub ist ein eigenes öffentliches Repository vorgesehen. Lege die Dateien aus diesem Ordner im Hauptzweig `main` ab. Der Workflow veröffentlicht beim Push von `VERSION` den Installer als Release und kann für die erste Veröffentlichung auch manuell unter **Actions → Windows-Release** gestartet werden. Die `VERSION`-Datei erhöht sich für jedes Update um einen Schritt. Auf 1.9 folgt 2.0. Der erste lokale Installer ist fest auf `TheGrifer1337/Medienregal` eingestellt. Bei abweichendem Reponamen installiere zuerst den dort veröffentlichten Installer.

Die App prüft beim Start alle öffentlichen stabilen Releases ihres Repositories und lädt die höchste neuere Version mit gültiger SHA-256-Prüfsumme. Wenn das Programm geschlossen wird, installiert es das fertig geladene Update im Hintergrund in denselben Ordner. Ein Programmneustart ist erst für die Nutzung der neuen Version nötig.
