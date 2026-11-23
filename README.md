# Mediathek by Maximilian

Du wolltest immer alle deine Medien Sortiert haben hier Bitte ein Praktische Mediathek mit vielen Funktionen :D

Mediathek by Maximilian ist eine persönliche Sammlung für Filme, Serien, Anime, Bücher und Manga. Die Windows-EXE startet in einem eigenen Windows-Fenster mit eingebetteter WebView2-Oberfläche. Falls die Laufzeit fehlt, installiert das Setup sie automatisch mit Internetverbindung. Die Sammlung liegt lokal unter `%APPDATA%\Medienregal\sammlung.json`. Für die Datensicherung kann diese Datei kopiert werden. Die Deinstallation über Windows behält die Sammlung.

## Suche und Streaming

Filme und Serien werden in Cinemeta gesucht, Bücher und Manga in der Deutschen Nationalbibliothek und Open Library. Bei Büchern kannst du Titel und Autor, Verlag und Reihe mit Bandnummer oder eine ISBN verwenden. Bei fehlenden Metadaten kannst du den Titel und die Gesamtzahl selbst eintragen oder korrigieren.

Unter **Alle Genres** stehen je Bereich gängige Genres und deine eigenen Einträge zur Auswahl. Beim Hinzufügen eines Buches schlägt die App anhand der Katalogangaben ein Hauptgenre vor. Dieses kannst du ändern oder bei fehlenden Katalogangaben selbst wählen. Weitere Genres bleiben als Filter verfügbar.

Für automatische Streamingangebote in „Möchte ich sehen“ trage unter **Einstellungen** einen Watchmode-API-Schlüssel für dein Land ein. Die App zeigt Abo, kostenlose Angebote, Leihen und Kaufen samt gemeldeten Preisen. Das vorgewählte Land ist Deutschland. Ein TMDB-Read-Access-Token ist optional und ergänzt Titel und Streaminganbieter. TMDB liefert hier keine Einzelpreise. Ohne Schlüssel verlinkt die App auf JustWatch. Anbieterangaben können sich ändern und bei Serien nur Teile der Serie abdecken.

Unter **Einstellungen → Meine Streaminganbieter** kannst du die von dir genutzten Dienste auswählen und weitere Namen ergänzen. In „Möchte ich sehen“ werden nur noch Angebote dieser Dienste angezeigt. **Nur bei meinen Anbietern** blendet auch Titel ohne bestätigtes passendes Angebot aus. Diese Filter brauchen Live-Angebote von Watchmode oder TMDB.

**Streamingverlauf importieren** liest den Netflix-Titelverlauf als CSV oder eingefügte Titel (einer pro Zeile) aus anderen Diensten ein. Für eigene CSV-Listen sind die Spalten `Title` oder `Titel` sowie optional `Typ` (`Film` oder `Serie`) verfügbar. Die Vorschau lässt dich Profil, Medientyp und Titel auswählen. Filme werden als „Angesehen“ und Serien als „Angefangen“ gespeichert. Wiederholte Titel werden pro Medientyp zusammengefasst. Einträge in „Möchte ich sehen“ werden bei eindeutigem Titel aktualisiert. Die App verarbeitet die Liste lokal. Unter **Einstellungen → Streamingimport rückgängig machen** zeigt eine Vorschau, welche Titel der letzte Import hinzugefügt hat und welche Statusänderungen zurückgesetzt werden. Bearbeitete Importtitel bleiben erhalten. Bei Importen aus älteren Versionen lässt sich nur die Gruppe unveränderter, neu hinzugefügter Titel sicher erkennen.

This product uses the TMDB API but is not endorsed or certified by TMDB. Streamingdaten stammen von Watchmode und JustWatch (über TMDB). Katalogdaten stammen auch von Cinemeta, der Deutschen Nationalbibliothek und Open Library.

## Windows-Installer und Veröffentlichungen

Voraussetzungen für den lokalen Build unter Linux sind Go ab 1.23, Bash, cURL und NSIS.

```sh
bash build.sh TheGrifer1337/Medienregal
```

Der Standardordner ist `%LOCALAPPDATA%\Programs\Medienregal`. Im Installer kann ein anderer beschreibbarer Ordner gewählt werden. Die Installation erfolgt pro Windows-Benutzer und legt eine Verknüpfung auf dem Desktop sowie im Startmenü an. Die EXE und der Installer sind für Windows x64 gebaut.

Für GitHub ist ein eigenes öffentliches Repository vorgesehen. Lege die Dateien aus diesem Ordner im Hauptzweig `main` ab. Der Workflow veröffentlicht beim Push von `VERSION` den Installer als Release und kann für die erste Veröffentlichung auch manuell unter **Actions → Windows-Release** gestartet werden. Die `VERSION`-Datei erhöht sich für jedes Update um einen Schritt. Auf 1.9 folgt 2.0. Der erste lokale Installer ist fest auf `TheGrifer1337/Medienregal` eingestellt. Bei abweichendem Reponamen installiere zuerst den dort veröffentlichten Installer.

Die App prüft beim Start und über **Einstellungen → Nach Updates suchen** alle öffentlichen stabilen Releases ihres Repositories und lädt die höchste neuere Version mit gültiger SHA-256-Prüfsumme. Wenn das Programm geschlossen wird, installiert es das fertig geladene Update im Hintergrund in denselben Ordner. Ein Programmneustart ist erst für die Nutzung der neuen Version nötig.
