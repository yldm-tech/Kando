// Kando's official AnkiConnect plugin (kando.ankiconnect): an export target of the Saved Lines sheet that sends the saved lines and the words of the vocabulary as notes to Anki on this computer, through the AnkiConnect add-on (https://git.foosoft.net/alex/anki-connect).
//
// What it may reach is what kando-plugin.json declares and the user granted: POST http://127.0.0.1:8765/, AnkiConnect's one endpoint, and the sound of a saved line (kando.sound.read, optional). AnkiConnect takes every action at that endpoint, deleting a deck included, so the rule cannot be narrowed to the actions used here; the plugin asks for nothing that reads the subtitles as they play or the AI service, so what it sends is decided by this signed code and the lines the user chose to export, never by a subtitle's text (docs/plugin-system.md 4.2, 6.2).
//
// A failure the user can act on (Anki not running, AnkiConnect too old or refusing Kando, a note type that does not fit) rejects with an Error carrying a code, which Kando shows under the plugin's name and does not count against the plugin. The functions Kando calls are async on purpose: a synchronous throw loses its code (nsgo#847).

import { host } from "nsgo"
import manifest from "./kando-plugin.json"

// The signed code says what the plugin is: Kando compares this with kando-plugin.json, which the signature does not cover (docs/plugin-system.md 4.3). It is the manifest itself, compiled into the signed bundle, so the two cannot drift apart.
const signedKeys = ["capabilities", "optional", "contributes", "credentials", "settings", "events"]

export function kandoManifest() {
  const said = {}
  for (const key of signedKeys) {
    if (key in manifest) said[key] = manifest[key]
  }
  return said
}

const endpoint = "http://127.0.0.1:8765/"
// AnkiConnect's API version this plugin speaks, the one that has requestPermission, canAddNotes and storeMediaFile.
const apiVersion = 6
// The most notes sent in one canAddNotes or addNotes.
const batchSize = 100

// Kando's runner stops a call that runs for more than a minute, the time Kando spends on the plugin's behalf (reading a sound) aside, and turns the plugin off as slow (docs/plugin-system.md 6.6). An export keeps well under it: its own time (requests to Anki, waits) is held to budgetMs, each request to requestMs, and one that cannot finish in what is left fails with anki-timeout, saying how many notes went in, so the user exports again and Anki skips those.
const budgetMs = 45000
const requestMs = 20000
// The least time a request is started with: below it, the export stops rather than start one that would be cut off.
const requestMinMs = 2000
// Kando answers at most 60 host calls of a plugin in a minute (quota); after the 60th sound of an export the plugin waits for the minute to pass when its budget allows it, and the lines after that go without sound, which the export reports.
const callsPerMinute = 60
// What a wait for the quota's minute must leave of the budget, for the requests still to come.
const quotaReserveMs = 15000
// How long a sound may take: Kando gives up on one after 120 s and refuses with "timeout".
const soundMs = 150000

// The note types the noteType setting chooses, by its choice id. Kando's own is made the first time it is missing; Basic's are Anki's stock ones, whose first two fields, whatever the collection's language named them, take the front and the back.
const noteTypes = {
  kando: "Kando",
  basic: "Basic",
  "basic-reversed": "Basic (and reversed card)",
}

const kandoFields = ["Text", "Meaning", "Reading", "Context", "Sound", "Source"]

const kandoModel = {
  modelName: "Kando",
  inOrderFields: kandoFields,
  isCloze: false,
  css: ".card { font-family: sans-serif; font-size: 22px; text-align: center; color: black; background-color: white; }\n.reading { font-size: 16px; color: #666; }\n.context, .source { font-size: 15px; color: #888; margin-top: 12px; }\n.night_mode .card { color: white; background-color: #222; }",
  cardTemplates: [
    {
      Name: "Card 1",
      Front: "{{Text}}<br>{{Sound}}",
      Back: '{{FrontSide}}<hr id="answer"><div class="reading">{{Reading}}</div>{{Meaning}}<div class="context">{{Context}}</div><div class="source">{{Source}}</div>',
    },
  ],
}

// fail is an Error with a code, which Kando shows under the plugin's name and its circuit breaker does not count.
function fail(code: string, message: string): Error {
  const e = new Error(message) as Error & { code: string }
  e.code = code
  return e
}

// Export is one export's clock and tallies: the time it spent of its own, which is all of it but the waits for Kando to read a sound (hostMs), how many notes went in so far, and the state of the sounds: whether to ask for them still, the times it asked (for the quota), and how many lines went in without one.
class Export {
  start = Date.now()
  hostMs = 0
  sent = 0
  sound: boolean
  asked: number[] = []
  soundless = 0

  constructor(sound: boolean) {
    this.sound = sound
  }

  // left is how much of the budget is left.
  left(): number {
    return budgetMs - (Date.now() - this.start - this.hostMs)
  }

  // tooSlow is the failure of an export that ran out of time, saying what went in.
  tooSlow(): Error {
    return fail("anki-timeout", `Anki is answering too slowly; ${this.sent} notes went in. Export again when Anki is idle to send the rest: it skips those it has.`)
  }
}

// sleep waits ms.
function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

// anki runs one AnkiConnect action and gives its result, or rejects with a coded Error: AnkiConnect not answering, or not within the request's time, a refused request, an answer that is not AnkiConnect's, or the error AnkiConnect returned.
async function anki(x: Export, action: string, params?: object): Promise<any> {
  const ms = Math.min(requestMs, x.left())
  if (ms < requestMinMs) throw x.tooSlow()
  const abort = new AbortController()
  const slow = fail("anki-timeout", `AnkiConnect did not answer within ${Math.round(ms / 1000)} s; Anki may be busy. ${x.sent} notes went in; export again to send the rest.`)
  const timer = setTimeout(() => abort.abort(slow), ms)
  try {
    let res
    try {
      res = await fetch(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action, version: apiVersion, params: params ?? {} }),
        signal: abort.signal,
      })
    } catch (e) {
      if (e === slow) throw slow
      // Kando refuses the request before it is sent when the user did not grant the network (ERR_ACCESS_DENIED); anything else is nothing listening on the port.
      if (e && (e.code === "ERR_ACCESS_DENIED" || e.permission)) {
        throw fail("no-network", "Kando does not let this plugin reach Anki: turn on its network permission in Settings › Plugins.")
      }
      throw fail("anki-unreachable", "Anki is not running, or the AnkiConnect add-on is not installed: nothing answers at 127.0.0.1:8765.")
    }
    if (!res.ok) {
      throw fail("anki-bad-reply", `AnkiConnect answered HTTP ${res.status}.`)
    }
    let reply
    try {
      reply = await res.json()
    } catch (e) {
      if (e === slow) throw slow
      throw fail("anki-bad-reply", "What answered at 127.0.0.1:8765 is not AnkiConnect.")
    }
    if (reply === null || typeof reply !== "object" || !("result" in reply) || !("error" in reply)) {
      throw fail("anki-bad-reply", "What answered at 127.0.0.1:8765 is not AnkiConnect.")
    }
    if (reply.error) {
      throw fail("anki-error", `AnkiConnect: ${String(reply.error).slice(0, 300)}`)
    }
    return reply.result
  } finally {
    clearTimeout(timer)
  }
}

// connect checks that AnkiConnect lets Kando in and speaks apiVersion or newer. requestPermission makes Anki ask the user when AnkiConnect does not know the caller; one that is too old to have it is told apart from one that refuses.
async function connect(x: Export): Promise<void> {
  let permission
  try {
    permission = await anki(x, "requestPermission")
  } catch (e) {
    if (e.code === "anki-error") {
      throw fail("anki-version", "AnkiConnect is too old for Kando: update the add-on in Anki (Tools › Add-ons › Check for Updates).")
    }
    throw e
  }
  if (!permission || permission.permission !== "granted") {
    throw fail("anki-denied", "AnkiConnect did not let Kando in: allow it when Anki asks, or in AnkiConnect's settings.")
  }
  if (permission.requireApikey) {
    throw fail("anki-api-key", "AnkiConnect asks for an API key, which this plugin does not send: clear apiKey in AnkiConnect's settings.")
  }
  const version = await anki(x, "version")
  if (typeof version !== "number" || version < apiVersion) {
    throw fail("anki-version", `AnkiConnect speaks version ${version}; Kando needs ${apiVersion} or newer. Update the add-on in Anki.`)
  }
}

// noteFields makes sure the note type exists and gives the names of its fields: Kando's own, made the first time it is missing, or a stock Basic one, whose first two fields are its front and back.
async function noteFields(x: Export, model: string): Promise<string[]> {
  const models = await anki(x, "modelNames")
  if (!Array.isArray(models) || !models.includes(model)) {
    if (model !== kandoModel.modelName) {
      throw fail("anki-note-type", `Anki has no note type named "${model}": choose Kando in the plugin's settings, which it makes for you.`)
    }
    await anki(x, "createModel", kandoModel)
    return kandoFields
  }
  const fields = await anki(x, "modelFieldNames", { modelName: model })
  if (!Array.isArray(fields)) {
    throw fail("anki-bad-reply", "AnkiConnect did not list the note type's fields.")
  }
  if (model === kandoModel.modelName ? !kandoFields.every((f) => fields.includes(f)) : fields.length < 2) {
    throw fail("anki-note-type", `The note type "${model}" in Anki does not have the fields Kando fills in: choose another one in the plugin's settings.`)
  }
  return fields
}

// escape is text as HTML, which Anki's fields are, its line breaks kept.
function escape(text: unknown): string {
  return String(text ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/\r?\n/g, "<br>")
}

// parts are what a saved line or a word puts in the note, as Kando's note type names its fields.
function parts(item: any): Record<string, string> {
  if (item.kind === "word") {
    const context = [item.context, item.contextTranslation].filter((t) => t).map(escape).join("<br>")
    return { Text: escape(item.text), Meaning: escape(item.meaning), Reading: escape(item.reading), Context: context, Sound: "", Source: escape(item.title) }
  }
  return { Text: escape(item.original), Meaning: escape(item.translation), Reading: "", Context: escape(item.note), Sound: "", Source: escape(item.title) }
}

// fieldsOf are the parts as the note type's fields: Kando's own one by one, a Basic one's text on the front and the rest, the sound included, on the back, so a sound file never makes two notes of the same text look different to Anki's duplicate check.
function fieldsOf(p: Record<string, string>, model: string, fields: string[]): Record<string, string> {
  if (model === kandoModel.modelName) {
    return Object.assign({}, p)
  }
  const back = [p.Reading, p.Meaning, p.Context, p.Sound, p.Source].filter((t) => t).join("<br>")
  return { [fields[0]]: p.Text, [fields[1]]: back }
}

// settingsOf are the plugin's settings as Kando gave them, with what is missing or empty at its default: the deck Kando, Kando's note type, the tag kando and the sound attached.
function settingsOf(s: any) {
  const deck = typeof s?.deck === "string" && s.deck.trim() !== "" ? s.deck.trim() : "Kando"
  const model = noteTypes[s?.noteType] ?? noteTypes.kando
  const tags = typeof s?.tags === "string" ? s.tags.split(/[\s,]+/).filter((t) => t) : []
  return { deck, model, tags: tags.length > 0 ? tags : ["kando"], sound: s?.sound !== false }
}

// mediaName is the file a saved line's sound is stored under in Anki's media folder: the same for the same line, so sending it again does not pile up copies.
function mediaName(id: string): string {
  return "kando-" + String(id).replace(/[^A-Za-z0-9_-]/g, "_").slice(0, 80) + ".m4a"
}

// readSound is the sound of the saved line id, base64, or null when it goes without one. A refusal that holds for every line (not granted, no ffmpeg, a sound that hung until Kando gave up) stops asking for the rest; one of this line's alone (no sound, too large, failed, another read in flight) skips it. Kando's quota of host calls is a minute's: past it the export waits for the minute to pass when its budget allows that, and asks again once; otherwise the rest go without sound. The time Kando spends reading is not the export's own (Export.hostMs), as it is not the runner's either.
async function readSound(x: Export, id: string): Promise<string | null> {
  if (!x.sound || !id) return null
  for (let attempt = 0; attempt < 2; attempt++) {
    const asked = Date.now()
    x.asked.push(asked)
    try {
      const clip = await host.call("kando.sound.read", { saved: id }, { timeout: soundMs })
      return clip && typeof clip.base64 === "string" && clip.base64 !== "" ? clip.base64 : null
    } catch (e) {
      const code = e?.code
      if (code === "quota" && attempt === 0) {
        const window = x.asked.filter((t) => t > Date.now() - 60000)
        const wait = window.length >= callsPerMinute ? window[window.length - callsPerMinute] + 60000 + 250 - Date.now() : 60250
        if (x.left() - wait > quotaReserveMs) {
          await sleep(wait)
          continue
        }
      }
      if (["quota", "ERR_ACCESS_DENIED", "denied", "no-ffmpeg", "timeout", "ETIMEDOUT"].includes(code)) {
        x.sound = false
      }
      return null
    } finally {
      x.hostMs += Date.now() - asked
    }
  }
  x.sound = false
  return null
}

// addOneByOne adds notes one at a time, for an AnkiConnect whose addNotes failed as a whole because one of them could not be added: it reports how many were.
async function addOneByOne(x: Export, notes: any[]): Promise<number> {
  let added = 0
  for (const note of notes) {
    try {
      if (await anki(x, "addNote", { note })) {
        added++
        x.sent++
      }
    } catch (e) {
      if (e.code !== "anki-error") throw e
    }
  }
  return added
}

// exportNotes is the export target "Anki (AnkiConnect)": it sends items, the saved lines and words the user exported, to the deck the settings name as notes of the note type they choose, the sound of each line attached when they ask for it, and answers how many it sent, how many it skipped (those Anki already has in the deck, canAddNotes; the same text twice in one export; items with nothing to show on the front) and, with the sound asked for, how many lines went in without one. Kando's runner allows a call a minute of its own time, so an export of more than Anki answers in that time fails with anki-timeout after sending what it could (Export).
export async function exportNotes(items: any[], settings: any) {
  if (!Array.isArray(items) || items.length === 0) {
    return { sent: 0, skipped: 0, soundless: 0 }
  }
  const s = settingsOf(settings)
  const x = new Export(s.sound)
  await connect(x)
  await anki(x, "createDeck", { deck: s.deck })
  const fields = await noteFields(x, s.model)
  const seen = new Set()
  let skipped = 0
  for (let start = 0; start < items.length; start += batchSize) {
    const batch = []
    for (const item of items.slice(start, start + batchSize)) {
      const p = parts(item ?? {})
      // Anki tells duplicates apart by the first field, which is the text in every note type here; the same text twice in one export is one note.
      if (p.Text === "" || seen.has(p.Text)) {
        skipped++
        continue
      }
      seen.add(p.Text)
      batch.push({ item, p })
    }
    if (batch.length === 0) continue
    const notes = batch.map(({ p }) => ({ deckName: s.deck, modelName: s.model, fields: fieldsOf(p, s.model, fields), tags: s.tags, options: { allowDuplicate: false, duplicateScope: "deck" } }))
    const can = await anki(x, "canAddNotes", { notes })
    if (!Array.isArray(can) || can.length !== notes.length) {
      throw fail("anki-bad-reply", "AnkiConnect did not say which notes it can add.")
    }
    const adding = []
    for (let i = 0; i < batch.length; i++) {
      if (!can[i]) {
        skipped++
        continue
      }
      const { item, p } = batch[i]
      // The sound is read and stored only for a line that is going in, never for a duplicate.
      if (item.kind !== "word") {
        const data = await readSound(x, item.id)
        if (data !== null) {
          const name = await anki(x, "storeMediaFile", { filename: mediaName(item.id), data })
          p.Sound = `[sound:${escape(typeof name === "string" && name !== "" ? name : mediaName(item.id))}]`
          notes[i].fields = fieldsOf(p, s.model, fields)
        }
      }
      adding.push({ note: notes[i], sound: p.Sound !== "", line: item.kind !== "word" })
    }
    if (adding.length === 0) continue
    let added
    try {
      const ids = await anki(x, "addNotes", { notes: adding.map((a) => a.note) })
      added = Array.isArray(ids) ? adding.filter((a, i) => ids[i] !== null && ids[i] !== undefined) : []
      x.sent += added.length
    } catch (e) {
      // Newer AnkiConnect fails addNotes as a whole when one note cannot be added, such as one added from elsewhere since canAddNotes; older ones answer null for it.
      if (e.code !== "anki-error") throw e
      added = []
      for (const a of adding) {
        if ((await addOneByOne(x, [a.note])) === 1) added.push(a)
      }
    }
    skipped += adding.length - added.length
    if (s.sound) {
      x.soundless += added.filter((a) => a.line && !a.sound).length
    }
  }
  return { sent: x.sent, skipped, soundless: x.soundless }
}
