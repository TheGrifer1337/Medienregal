# Mediathek by Maximilian

Du wolltest immer alle deine Medien Sortiert haben hier, bitte, eine praktische Mediathek mit vielen Funktionen :D

Mediathek by Maximilian ist eine persönliche Sammlung für Filme, Serien, Anime, Bücher und Manga. Die Windows-EXE startet in einem eigenen Windows-Fenster mit eingebetteter WebView2-Oberfläche. Falls die Laufzeit fehlt, installiert das Setup sie automatisch mit Internetverbindung. Filme, Serien, Bücher und Spiele werden lokal unter `%APPDATA%\Medienregal\sammlung.json` gespeichert. Zum Verwalten deiner Sammlung brauchst du keine Anmeldung. Katalogsuche, Streaminginformationen und Updates verwenden weiterhin eine Internetverbindung. Für die Datensicherung kann diese Datei kopiert werden. Die Deinstallation über Windows behält die Sammlung.

## Suche und Streaming

Filme und Serien werden in Cinemeta gesucht, Bücher und Manga in der Deutschen Nationalbibliothek und Open Library. Bei Büchern kannst du Titel und Autor, Verlag und Reihe mit Bandnummer oder eine ISBN verwenden. Bei fehlenden Metadaten kannst du den Titel und die Gesamtzahl selbst eintragen oder korrigieren.

Unter **Alle Genres** stehen je Bereich gängige Genres und deine eigenen Einträge zur Auswahl. Beim Hinzufügen eines Buches schlägt die App anhand der Katalogangaben ein Hauptgenre vor. Dieses kannst du ändern oder bei fehlenden Katalogangaben selbst wählen. Weitere Genres bleiben als Filter verfügbar.

Für automatische Streamingangebote in „Möchte ich sehen“ trage unter **Settings** einen Watchmode-API-Schlüssel für dein Land ein. Die App zeigt Abo, kostenlose Angebote, Leihen und Kaufen samt gemeldeten Preisen. Das vorgewählte Land ist Deutschland. Ein TMDB-Read-Access-Token ist optional und ergänzt Titel und Streaminganbieter. TMDB liefert hier keine Einzelpreise. Ohne Schlüssel verlinkt die App auf JustWatch. Anbieterangaben können sich ändern und bei Serien nur Teile der Serie abdecken.

Unter **Einstellungen → Meine Streaminganbieter** kannst du die von dir genutzten Dienste auswählen und weitere Namen ergänzen. In „Möchte ich sehen“ werden nur noch Angebote dieser Dienste angezeigt. **Nur bei meinen Anbietern** blendet auch Titel ohne bestätigtes passendes Angebot aus. Diese Filter brauchen Live-Angebote von Watchmode oder TMDB.

**Streamingverlauf importieren** liest den Netflix-Titelverlauf als CSV oder eingefügte Titel (einer pro Zeile) aus anderen Diensten ein. Für eigene CSV-Listen sind die Spalten `Title` oder `Titel` sowie optional `Typ` (`Film` oder `Serie`) verfügbar. Die Vorschau lässt dich Profil, Medientyp und Titel auswählen. Filme werden als „Angesehen“ und Serien als „Angefangen“ gespeichert. Wiederholte Titel werden pro Medientyp zusammengefasst. Einträge in „Möchte ich sehen“ werden bei eindeutigem Titel aktualisiert. Die App verarbeitet die Liste lokal. Unter **Einstellungen → Streamingimport rückgängig machen** zeigt eine Vorschau, welche Titel der letzte Import hinzugefügt hat und welche Statusänderungen zurückgesetzt werden. Bearbeitete Importtitel bleiben erhalten. Bei Importen aus älteren Versionen lässt sich nur die Gruppe unveränderter, neu hinzugefügter Titel sicher erkennen.

This product uses the TMDB API but is not endorsed or certified by TMDB. Streamingdaten stammen von Watchmode und JustWatch (über TMDB). Katalogdaten stammen auch von Cinemeta, der Deutschen Nationalbibliothek und Open Library.

## Sammlung und Jahreslisten

Titel können mehreren **eigenen Listen** angehören und persönliche Notizen speichern. Im Regal stehen Listenansicht, kleine und große Cover zur Wahl. **Duplikate** schlägt Paare über Titel, Originaltitel und Katalog-IDs vor. Die Funktion **Rückgängig** nimmt die letzte gespeicherte Änderung an einem Titel, eine Entfernung oder eine Zusammenführung zurück. Im Reiter **Statistik** findest du Abschlüsse, Genres und Bewertungen. Ältere Titel ohne erfasstes Abschlussjahr erscheinen unter „Alle Jahre“.

**In meinem Abo enthalten** zeigt in der Merkliste nur bestätigte Abonnement-Angebote deiner Streaminganbieter. Beim Katalogabgleich für importierte Titel werden eindeutige Ergebnisse automatisch ergänzt. Mehrere Treffer kannst du auswählen.

Die durchlaufende Jahresliste zeigt in jeder Medienart bis zu 50 Treffer für das eingestellte Land, geordnet nach der jeweiligen Katalogquelle. Verfügbare YouTube-Trailer starten in der Film- oder Serienvorschau. Beim Schließen wird der Player beendet. Ein Titel wird erst durch Antippen eines Status gespeichert.

Im Reiter **Spiele** kannst du Titel über Steam suchen oder selbst anlegen. Spiele lassen sich nach **Qualität**, **Gameplay** und **Storyline** bewerten. Die Spielecharts zeigen Bestseller des Jahres oder aktuelle Veröffentlichungen aus dem regionalen Steam-Shop. Die Quelle steht an der Leiste.

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

Die App prüft beim Start und über **Settings → Check for updates** alle öffentlichen stabilen Releases ihres Repositories und lädt die höchste neuere Version mit gültiger SHA-256-Prüfsumme. Wenn das Programm geschlossen wird, installiert es das fertig geladene Update im Hintergrund in denselben Ordner. Ein Programmneustart ist erst für die Nutzung der neuen Version nötig.

### Automatische Coversuche

Fehlt das Cover eines sichtbaren Titels oder lässt sich das Bild nicht laden, sucht die Mediathek nach passenden Bildern in weiteren Online-Katalogen. Je nach Titelart nutzt sie Google Books und Open Library, TVmaze, TMDB (mit hinterlegtem Token), MetaHub, Cinemeta, Steam und Wikimedia Commons. Bekannte Kennungen wie ISBN oder IMDb-ID helfen beim Abgleich. Eigene Cover werden zuerst angezeigt. In der Detailansicht steht die Quelle eines gefundenen Ersatzcovers.

### Wiedergabe, Reihen, Jahresziele und Verleih

**Ansehen** zeigt die Abspiel- und Angebotslinks für Filme und Serien. Watchmode liefert Anbieterlinks, soweit sie für den Titel und das Land gemeldet werden. Mit TMDB oder ohne Streaming-API führt die Mediathek bei Bedarf zu JustWatch. **Spiel starten** verwendet für lokal erkannte Titel Steam, den Epic Games Launcher oder einen eindeutigen registrierten Xbox-Startlink unter Windows.

Speichere in der Detailansicht unter **Reihe und Verleih** den Namen einer Reihe oder eines Universums und eine Position. Die Übersicht **Reihen und Universen** sortiert die zugeordneten Titel medienübergreifend. Bei Filmen mit TMDB-Daten und hinterlegtem Token steht zusätzlich die Veröffentlichungsreihenfolge einer katalogisierten Filmreihe zur Verfügung. Zur selben Detailansicht gehören ausgeliehene Medien, Empfänger und optionales Leih- und Rückgabedatum. **Verliehene Medien** sammelt diese Einträge in einer Übersicht.

Unter **Statistik → Jahresziele** kann jede Medienart pro Jahr ein eigenes Ziel erhalten. Der Fortschritt zählt abgeschlossene Titel mit dem gewählten Abschlussjahr. Eine leere Eingabe entfernt das Ziel.

### Barcode, IMDb und Trailer

Beim Hinzufügen eines Titels liest **ISBN oder Barcode scannen** EAN- und UPC-Codes per Webcam oder von einem Foto. Eine manuelle Eingabe ist ebenfalls möglich. Die Bilder werden lokal verarbeitet. ISBNs von Büchern gehen in die Buchsuche, Produktbarcodes werden bei UPCitemdb nachgeschlagen. Die Ergebnisse müssen vor der Übernahme ausgewählt werden.

Filmkarten und Filmcharts zeigen die verfügbare IMDb-Wertung auf einer Skala bis 10. In der Detailansicht gespeicherter Filme lässt sich ein verfügbarer Trailer ansehen oder ein eigener YouTube-Link speichern.

### Land und Sprache

**Settings → App language** bietet Deutsch, Englisch und vereinfachtes Chinesisch. **Settings** bleibt auf Englisch. Externe Kataloge und selbst eingetragene Inhalte liefern ihren eigenen Text. Die App fragt lokalisierte Metadaten dort ab, wo der Anbieter sie unterstützt.

**Settings → Country for streaming and charts** legt das Land für Streamingangebote und Charts fest. Film- und Seriencharts nutzen bei verfügbarem TMDB-Zugang den Streamingmarkt und sonst Produktionen des gewählten Landes. Bücher werden in Open Library nach der zugeordneten Hauptsprache des Landes gefiltert, und Spiele verwenden den regionalen Steam-Shop. Die Quelle und der jeweilige Filter stehen an der Rangliste.

### Entdecken, Steam und Export

Der Bereich **Für dich entdeckt** am Rand nutzt Favoriten, Bewertungen, Genres und bei Büchern auch Autorennamen. **Settings** nimmt SteamID64 und einen persönlichen Steam-Web-API-Schlüssel entgegen. Für zugängliche Steam-Spiele zeigt die Mediathek Spielzeit und, sofern von Steam geliefert, den letzten Spieltermin. Vor dem Öffnen der Wiedergabe oder dem Spielstart werden verfügbare landesspezifische Altersfreigaben und Links zu Inhaltswarnungen angezeigt. Bei Filmwertungen wird die eigene 5-Punkte-Wertung für den Abweichungsfilter auf 10 Punkte umgerechnet.

**Liste exportieren** erstellt eine HTML-Datei mit Titel, Cover und eigener Wertung. Eingebettete eigene Cover stehen direkt in der Datei, andere Cover werden über ihre Bildadresse geladen. Der Export enthält keine Notizen, Zugangsdaten oder Leihdaten.
