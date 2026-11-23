import BackgroundTasks
import SwiftUI

@MainActor
@main
struct MediathekApp: App {
    @Environment(\.scenePhase) var phase
    @StateObject private var library = LibraryStore()
    private let appRefresh = "de.maximilian.mediathek.refresh"

    var body: some Scene {
        WindowGroup {
            LibraryScreen(library: library)
                .onChange(of: phase, initial: true) { _, phase in
                    if phase == .active { Task { await library.sync() } }
                    if phase == .background { scheduleRefresh() }
                }
                .task {
                    while !Task.isCancelled {
                        do { try await Task.sleep(for: .seconds(25)) }
                        catch { break }
                        if phase == .active { await library.sync() }
                    }
                }
        }
        .backgroundTask(.appRefresh(appRefresh)) {
            await MainActor.run { scheduleRefresh() }
            await library.sync()
        }
    }

    private func scheduleRefresh() {
        let request = BGAppRefreshTaskRequest(identifier: appRefresh)
        request.earliestBeginDate = Date(timeIntervalSinceNow: 15 * 60)
        try? BGTaskScheduler.shared.submit(request)
    }
}

@MainActor
struct LibraryScreen: View {
    @ObservedObject var library: LibraryStore
    @State private var kind = MediaKind.movie
    @State private var query = ""
    @State private var account = false
    @State private var statusFilter = ""
    @State private var edit: EditTarget?

    private var items: [Media] {
        library.state.visible.filter { item in
            item.kind == kind && (statusFilter.isEmpty || item.status == statusFilter) &&
                (query.isEmpty || item.title.localizedStandardContains(query) || (item.subtitle ?? "").localizedStandardContains(query))
        }
    }

    var body: some View {
        NavigationStack {
            if library.connection == nil {
                AccountForm(library: library)
                    .navigationTitle("Mediathek")
            } else {
                List {
                    syncStatus
                    Picker("Medienart", selection: $kind) {
                        ForEach(MediaKind.allCases) { Text($0.title).tag($0) }
                    }
                    .pickerStyle(.segmented)
                    .onChange(of: kind) { statusFilter = "" }
                    ForEach(items) { item in
                        Button {
                            edit = EditTarget(media: item, original: item)
                        } label: {
                            HStack(spacing: 12) {
                                Image(systemName: item.kind.symbol)
                                    .frame(width: 34, height: 37)
                                    .foregroundStyle(.brown)
                                VStack(alignment: .leading) {
                                    Text(item.title).font(.headline)
                                    if let subtitle = item.subtitle, !subtitle.isEmpty { Text(subtitle).font(.caption).foregroundStyle(.secondary) }
                                    Text(item.statusLabel).font(.caption).foregroundStyle(.secondary)
                                }
                                Spacer()
                                if item.favorite == true { Image(systemName: "heart.fill").foregroundStyle(.red) }
                            }
                            .foregroundStyle(.primary)
                        }
                    }
                    if items.isEmpty {
                        ContentUnavailableView(query.isEmpty ? "Dein Regal ist leer" : "Keine Treffer",
                                               systemImage: kind.symbol,
                                               description: Text("Füge mit + einen Titel hinzu oder ändere den Filter."))
                    }
                }
                .searchable(text: $query, prompt: "In der Sammlung suchen")
                .navigationTitle("Mediathek")
                .refreshable { await library.sync() }
                .toolbar {
                    ToolbarItem(placement: .topBarLeading) {
                        Button { account = true } label: { Label("Konto", systemImage: "person.crop.circle") }
                    }
                    ToolbarItemGroup(placement: .topBarTrailing) {
                        Menu {
                            Button("Alle Status") { statusFilter = "" }
                            ForEach(kind.statuses, id: \.self) { status in
                                Button(statusLabel(status)) { statusFilter = status }
                            }
                        } label: { Label("Status filtern", systemImage: "line.3.horizontal.decrease.circle") }
                        Button {
                            edit = EditTarget(media: .blank(kind: kind), original: nil)
                        } label: { Label("Titel hinzufügen", systemImage: "plus") }
                    }
                }
            }
        }
        .sheet(isPresented: $account) {
            NavigationStack {
                AccountForm(library: library)
                    .navigationTitle("Konto")
                    .toolbar { ToolbarItem(placement: .cancellationAction) { Button("Schließen") { account = false } } }
            }
        }
        .sheet(item: $edit) { target in
            NavigationStack { MediaEditor(library: library, target: target) }
        }
    }

    @ViewBuilder private var syncStatus: some View {
        if library.syncing { Label("Sammlung wird abgeglichen", systemImage: "arrow.triangle.2.circlepath") }
        if !library.state.pending.isEmpty {
            Text("\(library.state.pending.count) Änderungen warten auf den Server").foregroundStyle(.secondary)
        }
        if !library.message.isEmpty { Text(library.message).foregroundStyle(.secondary) }
        if library.needsLogin { Button("Erneut anmelden") { account = true } }
        if !library.state.conflicts.isEmpty {
            Section("Änderungen auf beiden Geräten") {
                ForEach(library.state.conflicts) { conflict in
                    VStack(alignment: .leading, spacing: 8) {
                        Text(conflict.current?.title ?? "Titel wurde am Server gelöscht")
                        Button("Meine Änderung anwenden") { resolve(conflict, keepLocal: true) }
                        Button("Serverstand übernehmen") { resolve(conflict, keepLocal: false) }
                    }
                    .buttonStyle(.borderless)
                }
            }
        }
    }

    private func resolve(_ conflict: MergeConflict, keepLocal: Bool) {
        do { try library.resolve(conflict, keepLocal: keepLocal) }
        catch { library.message = error.localizedDescription }
    }
}

private func statusLabel(_ status: String) -> String {
    switch status {
    case "seen": "Gesehen"
    case "started": "Angefangen"
    case "finished": "Abgeschlossen"
    case "read": "Gelesen"
    case "library": "In der Sammlung"
    default: "Merkliste"
    }
}

private struct EditTarget: Identifiable {
    let media: Media
    let original: Media?
    var id: String { media.id }
}

@MainActor
private struct MediaEditor: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var library: LibraryStore
    let target: EditTarget
    @State private var item: Media
    @State private var genres: String
    @State private var message = ""
    @State private var confirmDeletion = false

    init(library: LibraryStore, target: EditTarget) {
        self.library = library
        self.target = target
        _item = State(initialValue: target.media)
        _genres = State(initialValue: target.media.genres?.joined(separator: ", ") ?? "")
    }

    var body: some View {
        Form {
            Section("Titel") {
                TextField("Titel", text: $item.title)
                TextField(item.kind == .book ? "Autor oder Autorin" : "Jahr oder Zusatz",
                          text: Binding(get: { item.subtitle ?? "" }, set: { item.subtitle = $0.isEmpty ? nil : $0 }))
                Picker("Status", selection: $item.status) {
                    ForEach(item.kind.statuses, id: \.self) { Text(statusLabel($0)).tag($0) }
                }
                Toggle("Favorit", isOn: Binding(get: { item.favorite ?? false }, set: { item.favorite = $0 }))
                TextField("Genres, durch Kommas getrennt", text: $genres)
            }
            if item.kind == .book || item.kind == .series {
                Section("Fortschritt") {
                    TextField("Bisher", value: $item.progress, format: .number).keyboardType(.numberPad)
                    TextField("Insgesamt", value: $item.total, format: .number).keyboardType(.numberPad)
                }
            }
            if item.ratings.count == 3 {
                Section("Bewertung") {
                    ForEach(0..<3, id: \.self) { index in
                        Stepper("\(item.kind.criteria[index]): \(Double(item.ratings[index]) / 2, format: .number) / 5",
                                value: $item.ratings[index], in: 0...10)
                    }
                }
            }
            if !library.state.lists.isEmpty {
                Section("Eigene Listen") {
                    ForEach(library.state.lists, id: \.self) { list in
                        Toggle(list, isOn: Binding(get: { item.lists?.contains(list) ?? false }, set: { on in
                            var names = item.lists ?? []
                            names.removeAll { $0 == list }
                            if on { names.append(list) }
                            item.lists = names
                        }))
                    }
                }
            }
            Section("Notizen") {
                TextField("Persönliche Notizen", text: Binding(get: { item.notes ?? "" }, set: { item.notes = $0.isEmpty ? nil : $0 }), axis: .vertical)
            }
            if !message.isEmpty { Text(message).foregroundStyle(.red) }
            if target.original != nil {
                Button("Titel löschen", role: .destructive) { confirmDeletion = true }
            }
        }
        .navigationTitle(target.original == nil ? "Neuer Titel" : "Titel bearbeiten")
        .toolbar {
            ToolbarItem(placement: .cancellationAction) { Button("Abbrechen") { dismiss() } }
            ToolbarItem(placement: .confirmationAction) { Button("Speichern", action: save).disabled(item.title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty) }
        }
        .confirmationDialog("Titel löschen?", isPresented: $confirmDeletion, titleVisibility: .visible) {
            Button("Löschen", role: .destructive) {
                do { try library.delete(target.original ?? item); dismiss() }
                catch { message = error.localizedDescription }
            }
        }
    }

    private func save() {
        guard item.progress >= 0, item.total >= 0, item.total == 0 || item.progress <= item.total else {
            message = "Prüfe deinen Fortschritt und die Gesamtanzahl."
            return
        }
        let selected = genres.split(separator: ",").map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }.filter { !$0.isEmpty }
        item.genres = selected.isEmpty ? nil : selected
        do { try library.save(item, original: target.original); dismiss() }
        catch { message = error.localizedDescription }
    }
}

@MainActor
struct AccountForm: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var library: LibraryStore
    @State private var server = ""
    @State private var name = ""
    @State private var password = ""
    @State private var invite = ""
    @State private var register = false

    var body: some View {
        Form {
            if let connection = library.connection, !library.needsLogin {
                Section("Angemeldet") {
                    Text(connection.name)
                    Text(connection.server.absoluteString).font(.footnote)
                    Button("Jetzt synchronisieren") { Task { await library.sync() } }
                    Button("Abmelden", role: .destructive) { Task { await library.signOut() } }
                        .disabled(!library.state.pending.isEmpty)
                }
            } else {
                Section("Mediathek-Konto") {
                    TextField("https://Kontoserver", text: $server)
                        .keyboardType(.URL).autocorrectionDisabled().textInputAutocapitalization(.never)
                    TextField("Benutzername", text: $name)
                        .textContentType(.username).autocorrectionDisabled().textInputAutocapitalization(.never)
                    SecureField("Passwort", text: $password)
                        .textContentType(register ? .newPassword : .password)
                    if register { TextField("Einladungscode", text: $invite).textInputAutocapitalization(.never) }
                    Toggle("Neues Konto erstellen", isOn: $register)
                        .disabled(library.needsLogin)
                    Button(register ? "Konto erstellen" : "Anmelden") {
                        Task {
                            await library.signIn(server: server, name: name, password: password, invite: invite, register: register)
                            if library.connection != nil && !library.needsLogin {
                                password = ""
                                dismiss()
                            }
                        }
                    }
                    .disabled(server.isEmpty || name.isEmpty || password.count < 10 || (register && invite.isEmpty) || library.syncing)
                    Text("Nach dem ersten Anmelden bleibt deine Sammlung auch ohne Internet auf dem iPhone verfügbar.")
                        .font(.footnote).foregroundStyle(.secondary)
                }
            }
            if !library.message.isEmpty { Text(library.message).foregroundStyle(.secondary) }
        }
        .onAppear {
            server = library.connection?.server.absoluteString ?? ""
            name = library.connection?.name ?? ""
        }
    }
}
