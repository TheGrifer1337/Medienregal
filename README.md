# Mediathek by Maximilian

Du wolltest immer alle deine Medien Sortiert haben hier, bitte, eine praktische Mediathek mit vielen Funktionen :D

Mediathek by Maximilian ist eine persönliche Sammlung für Filme, Serien, Anime, Bücher und Manga. Die Windows-EXE startet in einem eigenen Windows-Fenster mit eingebetteter WebView2-Oberfläche. Falls die Laufzeit fehlt, installiert das Setup sie automatisch mit Internetverbindung. Die Sammlung liegt lokal unter `%APPDATA%\Medienregal\sammlung.json`. Für die Datensicherung kann diese Datei kopiert werden. Die Deinstallation über Windows behält die Sammlung.

## Suche und Streaming

Filme und Serien werden in Cinemeta gesucht, Bücher und Manga in der Deutschen Nationalbibliothek und Open Library. Bei Büchern kannst du Titel und Autor, Verlag und Reihe mit Bandnummer oder eine ISBN verwenden. Bei fehlenden Metadaten kannst du den Titel und die Gesamtzahl selbst eintragen oder korrigieren.

Unter **Alle Genres** stehen je Bereich gängige Genres und deine eigenen Einträge zur Auswahl. Beim Hinzufügen eines Buches schlägt die App anhand der Katalogangaben ein Hauptgenre vor. Dieses kannst du ändern oder bei fehlenden Katalogangaben selbst wählen. Weitere Genres bleiben als Filter verfügbar.

Für automatische Streamingangebote in „Möchte ich sehen“ trage unter **Einstellungen** einen Watchmode-API-Schlüssel für dein Land ein. Die App zeigt Abo, kostenlose Angebote, Leihen und Kaufen samt gemeldeten Preisen. Das vorgewählte Land ist Deutschland. Ein TMDB-Read-Access-Token ist optional und ergänzt Titel und Streaminganbieter. TMDB liefert hier keine Einzelpreise. Ohne Schlüssel verlinkt die App auf JustWatch. Anbieterangaben können sich ändern und bei Serien nur Teile der Serie abdecken.

Unter **Einstellungen → Meine Streaminganbieter** kannst du die von dir genutzten Dienste auswählen und weitere Namen ergänzen. In „Möchte ich sehen“ werden nur noch Angebote dieser Dienste angezeigt. **Nur bei meinen Anbietern** blendet auch Titel ohne bestätigtes passendes Angebot aus. Diese Filter brauchen Live-Angebote von Watchmode oder TMDB.

**Streamingverlauf importieren** liest den Netflix-Titelverlauf als CSV oder eingefügte Titel (einer pro Zeile) aus anderen Diensten ein. Für eigene CSV-Listen sind die Spalten `Title` oder `Titel` sowie optional `Typ` (`Film` oder `Serie`) verfügbar. Die Vorschau lässt dich Profil, Medientyp und Titel auswählen. Filme werden als „Angesehen“ und Serien als „Angefangen“ gespeichert. Wiederholte Titel werden pro Medientyp zusammengefasst. Einträge in „Möchte ich sehen“ werden bei eindeutigem Titel aktualisiert. Die App verarbeitet die Liste lokal. Unter **Einstellungen → Streamingimport rückgängig machen** zeigt eine Vorschau, welche Titel der letzte Import hinzugefügt hat und welche Statusänderungen zurückgesetzt werden. Bearbeitete Importtitel bleiben erhalten. Bei Importen aus älteren Versionen lässt sich nur die Gruppe unveränderter, neu hinzugefügter Titel sicher erkennen.

This product uses the TMDB API but is not endorsed or certified by TMDB. Streamingdaten stammen von Watchmode und JustWatch (über TMDB). Katalogdaten stammen auch von Cinemeta, der Deutschen Nationalbibliothek und Open Library.

## Sammlung und Jahreslisten

Titel können mehreren **eigenen Listen** angehören und persönliche Notizen speichern. Im Regal stehen Listenansicht, kleine und große Cover zur Wahl. **Duplikate** schlägt Paare über Titel, Originaltitel und Katalog-IDs vor. Die Funktion **Rückgängig** nimmt die letzte gespeicherte Änderung an einem Titel, eine Entfernung oder eine Zusammenführung zurück. Im Reiter **Statistik** findest du Abschlüsse, Genres und Bewertungen. Ältere Titel ohne erfasstes Abschlussjahr erscheinen unter „Alle Jahre“.

**In meinem Abo enthalten** zeigt in der Merkliste nur bestätigte Abonnement-Angebote deiner Streaminganbieter. Beim Katalogabgleich für importierte Titel werden eindeutige Ergebnisse automatisch ergänzt. Mehrere Treffer kannst du auswählen.

Die durchlaufende Jahresliste erscheint bei Filmen, Serien und Büchern. Sie zeigt bis zu 50 Treffer, geordnet nach der jeweiligen Katalogquelle. Die Liste für Filme und Serien nutzt TMDB bei eingerichtetem Zugang, sonst Cinemeta, Wikidata oder IMDbAPI. Bei Wikidata bestimmt die Anzahl verknüpfter Artikel die Reihenfolge. Bücher werden nach den Leselisten von Open Library gereiht. Verfügbare YouTube-Trailer starten in der Film- oder Serienvorschau. Beim Schließen wird der Player beendet. Ein Titel wird erst durch Antippen eines Status gespeichert.

Im Reiter **Spiele** kannst du Titel über Steam suchen oder selbst anlegen. Spiele lassen sich nach **Qualität**, **Gameplay** und **Storyline** bewerten. Die Spielecharts verwenden Wikidata oder die neuesten Veröffentlichungen auf Steam. Die Quelle steht an der Leiste.

Mit **Chart-Genre** kannst du die Jahresliste auf ein Genre eingrenzen. Beim Start und beim Wechsel des Medienbereichs werden zuerst alle Genres gezeigt. Die sichtbare Jahresliste lädt nach Ablauf ihres Zwischenspeichers erneut, sobald sie frei sichtbar ist. Eigene Listen sind direkt als Filter sichtbar. Das Herz auf jeder Titelkarte markiert Favoriten. Über **Eigenes Cover** im Titeldialog kannst du eine lokale Bilddatei oder HTTPS-Bildadresse wählen und später zur Kataloggrafik zurückkehren.

### Installierte Spiele importieren

Im Reiter **Spiele → Installierte Spiele importieren** scannt die Mediathek die auf diesem Windows-PC installierten Titel aus Steam, dem Epic Games Launcher und der Xbox-App. Die Vorschau lässt sich nach Launcher oder Titel filtern. Ausgewählte Titel erhalten beim Import den Status **In Bibliothek**. Bereits vorhandene Titel werden über ihre Katalog-ID oder ihren Namen erkannt. Der Import kann über **Rückgängig** zurückgenommen werden. Die Erkennung liest lokale Installationsdaten. Bibliotheken in Online-Konten ohne lokale Installation und Spiele auf der Xbox-Konsole sind davon ausgenommen.

## Windows-Installer und Veröffentlichungen

Voraussetzungen für den lokalen Build unter Linux sind Go ab 1.23, Bash, cURL und NSIS.

```sh
bash build.sh TheGrifer1337/Medienregal
```

Der Standardordner ist `%LOCALAPPDATA%\Programs\Medienregal`. Im Installer kann ein anderer beschreibbarer Ordner gewählt werden. Die Installation erfolgt pro Windows-Benutzer und legt eine Verknüpfung auf dem Desktop sowie im Startmenü an. Die EXE und der Installer sind für Windows x64 gebaut.

Für GitHub ist ein eigenes öffentliches Repository vorgesehen. Lege die Dateien aus diesem Ordner im Hauptzweig `main` ab. Der Workflow veröffentlicht beim Push von `VERSION` den Installer als Release und kann für die erste Veröffentlichung auch manuell unter **Actions → Windows-Release** gestartet werden. Die `VERSION`-Datei erhöht sich für jedes Update um einen Schritt. Auf 1.9 folgt 2.0. Der erste lokale Installer ist fest auf `TheGrifer1337/Medienregal` eingestellt. Bei abweichendem Reponamen installiere zuerst den dort veröffentlichten Installer.

Die App prüft beim Start und über **Einstellungen → Nach Updates suchen** alle öffentlichen stabilen Releases ihres Repositories und lädt die höchste neuere Version mit gültiger SHA-256-Prüfsumme. Wenn das Programm geschlossen wird, installiert es das fertig geladene Update im Hintergrund in denselben Ordner. Ein Programmneustart ist erst für die Nutzung der neuen Version nötig.
