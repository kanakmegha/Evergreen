/**
 * Everroot discovery model.
 *
 * The discovery journey is a deterministic, client-driven flow. Every catalogue
 * value comes from the Go API (`/api/v1/traditions`, `/api/v1/nakshatras`,
 * `/api/v1/traditions/{slug}/regions`) and every recommendation comes from
 * `POST /api/v1/discoveries`. Nothing cultural is generated on the client, and
 * nothing is invented when the reviewed dataset has no record for a selection.
 *
 * Personal answers (free text a user writes about their own meaning) never leave
 * the browser. Only `tradition`, `region` and `nakshatra` are ever sent.
 */

export type IntentionId =
  | 'personal'
  | 'remember'
  | 'culture'
  | 'future'
  | 'explore';

export interface Intention {
  id: IntentionId;
  label: string;
  description: string;
  icon: string;
}

export const INTENTIONS: Intention[] = [
  {
    id: 'personal',
    label: 'Something about me or my life',
    description: 'A quality you value, a beginning, or something you are working toward.',
    icon: 'sprout',
  },
  {
    id: 'remember',
    label: 'Someone I want to remember',
    description: 'A living tribute to a person, a place, or a time you hold dear.',
    icon: 'heart',
  },
  {
    id: 'culture',
    label: 'My culture, traditions, or beliefs',
    description: 'Explore what is recorded within a tradition, region, or practice.',
    icon: 'globe',
  },
  {
    id: 'future',
    label: 'A gift for the future',
    description: 'Something meant to outlast the moment it began in.',
    icon: 'gift',
  },
  {
    id: 'explore',
    label: "I'm curious and want to explore",
    description: 'Browse trees, stories, and cultural connections with no personal questions.',
    icon: 'compass',
  },
];

const INTENTION_MAP = new Map(INTENTIONS.map((i) => [i.id, i]));

/** Older links used different intent ids; keep them working. */
const LEGACY_INTENT_ALIASES: Record<string, IntentionId> = {
  celebrate: 'personal',
  grow: 'future',
  curious: 'explore',
  explore: 'explore',
  personal: 'personal',
  remember: 'remember',
  culture: 'culture',
  future: 'future',
};

export function resolveIntentionId(raw: string | null | undefined): IntentionId | null {
  if (!raw) return null;
  return LEGACY_INTENT_ALIASES[raw] ?? null;
}

export function intention(id: IntentionId | null): Intention | null {
  return id ? INTENTION_MAP.get(id) ?? null : null;
}

/**
 * A short, clearly-labelled personal reflection for each intention. This is
 * personal symbolism chosen by the user — never a cultural claim.
 */
export function personalReflection(id: IntentionId | null): string {
  switch (id) {
    case 'personal':
      return 'The meaning you described is yours. A tree can hold it as a private marker — a way to keep a quality or a goal in view as it grows.';
    case 'remember':
      return 'Remembering is a form of care. Whatever you wrote here belongs to you, and nothing about it has to be shared to remain meaningful.';
    case 'future':
      return 'Planting something for later is an act of trust in the future. The tree does not need to exist yet for the intention to be real.';
    case 'culture':
      return 'You chose to look at what is recorded rather than what could be assumed. That is the difference between tradition and invention.';
    default:
      return 'Curiosity is a good place to start. Follow what draws you and leave the rest.';
  }
}

/* ---------------------------------------------------------------------------
   Conditional follow-ups
--------------------------------------------------------------------------- */

export interface ChoiceOption {
  value: string;
  label: string;
  description?: string;
  icon?: string;
}

export interface FollowUpConfig {
  question: string;
  intro?: string;
  /** A single-choice question. When absent the step is free text. */
  choices?: ChoiceOption[];
  prompt?: string;
  placeholder?: string;
  hint?: string;
  quickPicks?: string[];
  /** An honest caveat shown beneath the step. */
  note?: string;
}

export const FOLLOW_UPS: Partial<Record<IntentionId, FollowUpConfig>> = {
  personal: {
    question: 'What would you like this tree to hold?',
    intro:
      'Anything you write here stays in this browser. Everroot does not send personal notes to the server.',
    prompt: 'A quality, a beginning, or something you are working toward',
    placeholder: 'e.g. Patience — for a year of learning to slow down.',
    hint: 'Optional. A few words are enough, and you can change this later.',
    quickPicks: ['A quality I value', 'A milestone or aspiration', 'A connection with nature'],
  },
  remember: {
    question: 'What qualities or memories would you like this tree to represent?',
    intro:
      'You can write a quality, a memory, or a name — or skip this. There is no need to describe anything painful.',
    prompt: 'A quality, a memory, or a name',
    placeholder: 'e.g. Quiet steadiness — the way she made room for everyone.',
    hint: 'Optional. Whatever you write stays on this device.',
    quickPicks: ['Gentleness', 'Strength', 'Curiosity', 'Home'],
  },
  culture: {
    question: 'Which kind of tradition is closest to what you have in mind?',
    intro: "There's no wrong answer, and you can change direction at any time.",
    choices: [
      {
        value: 'nakshatra',
        label: 'Nakshatra or astrological traditions',
        description: 'Explore the tree associations recorded for the 27 nakshatras.',
        icon: 'sun',
      },
      {
        value: 'family',
        label: 'Family or regional customs',
        description: 'Traditions passed down in a family, a region, or a language.',
        icon: 'leaf',
      },
      {
        value: 'faith',
        label: 'Religious or spiritual traditions',
        description: 'Trees held sacred or meaningful within a particular practice.',
        icon: 'book',
      },
      {
        value: 'unsure',
        label: "I'm not sure yet",
        description: 'Look at what is on record and decide later.',
        icon: 'compass',
      },
    ],
  },
  future: {
    question: 'Who or what is this for?',
    intro:
      'Describe it in your own words, or just pick a starting point. This stays on your device.',
    prompt: 'The person, place, or idea you want this to stand for',
    placeholder: 'e.g. For my niece, so she has something older than she is.',
    hint: 'Optional.',
    quickPicks: ['A child or grandchild', 'A community or place', 'A promise to the future'],
    note: 'Everroot does not yet work with a verified planting partner, so choosing a tree here never plants one. What you can explore is the symbolism and the records.',
  },
};

export function followUpOptionLabel(
  id: IntentionId | null,
  choice: string | null
): string | null {
  if (!id || !choice) return null;
  const options = FOLLOW_UPS[id]?.choices;
  return options?.find((o) => o.value === choice)?.label ?? null;
}

/* ---------------------------------------------------------------------------
   Catalog
--------------------------------------------------------------------------- */

export interface Tradition {
  slug: string;
  name: string;
}

export interface Nakshatra {
  name: string;
  ordinal: number;
}

export const NAKSHATRA_TRADITION = 'nakshatra-tree-association';

/**
 * apiURL resolves a path against PUBLIC_API_URL if configured in production,
 * or falls back to a same-origin relative path.
 */
export function apiURL(path: string): string {
  const base = typeof import.meta !== 'undefined' && (import.meta as any).env?.PUBLIC_API_URL
    ? String((import.meta as any).env.PUBLIC_API_URL).replace(/\/+$/, '')
    : '';
  return `${base}${path}`;
}

async function getJSON<T>(url: string): Promise<T> {
  const res = await fetch(apiURL(url), { headers: { accept: 'application/json' } });
  if (!res.ok) throw new Error(`Request failed (${res.status})`);
  return (await res.json()) as T;
}

export function fetchTraditions(): Promise<Tradition[]> {
  return getJSON<Tradition[]>('/api/v1/traditions');
}

export function fetchNakshatras(): Promise<Nakshatra[]> {
  return getJSON<Nakshatra[]>('/api/v1/nakshatras');
}

export function fetchRegions(traditionSlug: string): Promise<string[]> {
  return getJSON<string[]>(
    `/api/v1/traditions/${encodeURIComponent(traditionSlug)}/regions`
  );
}

/**
 * Read-only fallback used only for result links created by the earlier
 * conversation-based flow. Returns null when the session is unknown or expired.
 */
export interface ConversationSession {
  id: string;
  intent?: string;
  tradition_slug?: string;
  region_name?: string;
  nakshatra?: string;
  state: string;
  expires_at: string;
}

export async function fetchSession(sessionId: string): Promise<ConversationSession | null> {
  try {
    const res = await fetch(
      apiURL(`/api/v1/conversation?session_id=${encodeURIComponent(sessionId)}`),
      { headers: { accept: 'application/json' } }
    );
    if (!res.ok) return null;
    return (await res.json()) as ConversationSession;
  } catch {
    return null;
  }
}

/* ---------------------------------------------------------------------------
   Discovery
--------------------------------------------------------------------------- */

export interface DiscoverySource {
  title: string;
  url: string;
  locator: string;
  relationship: string;
  bibliographicReference: string;
  attributionRequirements: string;
  reuseStatus: string;
  verificationStatus: string;
}

export interface BotanicalCandidate {
  candidateId?: string;
  scientificName: string;
  certainty: string;
  confidence: number;
  reviewStatus: string;
  definitive: boolean;
  evidence: DiscoverySource[];
}

export interface Recommendation {
  mappingId: string;
  revisionId: string;
  version: number;
  tradition: string;
  region: string;
  nakshatra: string;
  claim: string;
  interpretation: string;
  sources: DiscoverySource[];
  botanicalCandidates: BotanicalCandidate[];
}

export interface DiscoveryResponse {
  status: 'verified_results' | 'no_verified_mapping' | string;
  recommendations: Recommendation[];
}

export interface DiscoveryRequestInput {
  tradition: string;
  region: string;
  nakshatra: string;
}

export async function runDiscovery(input: DiscoveryRequestInput): Promise<DiscoveryResponse> {
  let res: Response;
  try {
    res = await fetch(apiURL('/api/v1/discoveries'), {
      method: 'POST',
      headers: { 'content-type': 'application/json', accept: 'application/json' },
      body: JSON.stringify(input),
    });
  } catch {
    throw new Error('The discovery service could not be reached from this browser.');
  }
  if (!res.ok) {
    throw new Error(
      res.status >= 500
        ? `The discovery service reported a server error (${res.status}).`
        : `The discovery service rejected the request (${res.status}).`
    );
  }
  const data = (await res.json()) as Partial<DiscoveryResponse>;
  return {
    status: data.status ?? 'no_verified_mapping',
    recommendations: Array.isArray(data.recommendations) ? data.recommendations : [],
  };
}

/* ---------------------------------------------------------------------------
   Exploratory botanical discovery (Track B)
--------------------------------------------------------------------------- */

/** One vernacular name, exactly as the authority dataset supplies it. */
export interface TreeCommonName {
  name: string;
  language: string;
}

/** The taxon record every botanical fact came from. */
export interface TreeAuthority {
  dataset: string;
  doi: string;
  url: string;
  taxonKey: number;
}

/**
 * One entry in Everroot's botanical reference catalogue: authority-verified
 * identity, classification and nomenclatural provenance only. It carries no
 * habitat, ecological or cultural claim, because Everroot has no reviewed
 * source for those.
 */
export interface TreeFact {
  speciesId: string;
  acceptedName: string;
  authorship: string;
  family: string;
  genus: string;
  order: string;
  rank: string;
  taxonStatus: string;
  protologue: string;
  commonNames: TreeCommonName[];
  authority: TreeAuthority;
  sources: DiscoverySource[];
  /** Always "botanical_authority". */
  recordType: string;
  /** Always "none". A botanical record is never a cultural claim. */
  culturalClaim: string;
}

/**
 * Which parts of a selection rest on a reviewed record. Structured, so the
 * interface can never turn a provider failure into a missing-data message.
 */
export interface EvidenceStatus {
  culturalAssociation: 'reviewed_match' | 'no_reviewed_match' | 'not_queryable' | string;
  botanicalIdentity: string;
  habitat: string;
  ecologicalRole: string;
  documentedStories: string;
  personalMeaning: string;
}

export interface ExplanationEnvelope {
  status: 'ok' | 'unavailable' | string;
  reason: string;
  text: string;
  model: string;
}

export interface CatalogueResponse {
  status: string;
  ordering: string;
  query: string;
  count: number;
  trees: TreeFact[];
}

export interface ExplainResponse {
  status: string;
  task: string;
  evidence: EvidenceStatus;
  trees: TreeFact[];
  explanation: ExplanationEnvelope;
}

export interface ExplainInput {
  intention: IntentionId;
  tradition: string;
  nakshatra: string;
  region: string;
  speciesId: string;
}

export async function fetchCatalogue(query = ''): Promise<CatalogueResponse> {
  const suffix = query ? `?q=${encodeURIComponent(query)}` : '';
  let res: Response;
  try {
    res = await fetch(apiURL(`/api/v1/trees${suffix}`), { headers: { accept: 'application/json' } });
  } catch {
    throw new Error('The botanical reference could not be reached from this browser.');
  }
  if (!res.ok) {
    throw new Error(
      res.status === 503
        ? 'The botanical reference is temporarily unavailable.'
        : `The botanical reference returned an error (${res.status}).`
    );
  }
  const data = (await res.json()) as Partial<CatalogueResponse>;
  return {
    status: data.status ?? 'ok',
    ordering: data.ordering ?? '',
    query: data.query ?? query,
    count: typeof data.count === 'number' ? data.count : 0,
    trees: Array.isArray(data.trees) ? data.trees : [],
  };
}

export async function runExplain(input: ExplainInput): Promise<ExplainResponse> {
  let res: Response;
  try {
    res = await fetch(apiURL('/api/v1/explain'), {
      method: 'POST',
      headers: { 'content-type': 'application/json', accept: 'application/json' },
      body: JSON.stringify(input),
    });
  } catch {
    throw new Error('The explanation service could not be reached from this browser.');
  }
  if (!res.ok) {
    throw new Error(
      res.status === 404
        ? 'Everroot has no botanical record for that tree.'
        : res.status >= 500
          ? `The explanation service reported a server error (${res.status}).`
          : `The request was rejected (${res.status}).`
    );
  }
  const data = (await res.json()) as Partial<ExplainResponse>;
  return {
    status: data.status ?? 'ok',
    task: data.task ?? '',
    evidence: data.evidence ?? {
      culturalAssociation: 'not_queryable',
      botanicalIdentity: 'unavailable',
      habitat: 'no_reviewed_record',
      ecologicalRole: 'no_reviewed_record',
      documentedStories: 'no_reviewed_record',
      personalMeaning: 'user_owned',
    },
    trees: Array.isArray(data.trees) ? data.trees : [],
    explanation: data.explanation ?? { status: 'unavailable', reason: 'provider_error', text: '', model: '' },
  };
}

/**
 * Honest copy for a provider state. These strings never suggest that cultural
 * records are missing — a provider problem is a provider problem.
 */
/**
 * Plain-language text for a provider state. It always says the provider failed
 * and never that a record is missing: a dead or unconfigured provider must not
 * read as "there is no cultural association".
 *
 * `scope` only changes the closing reassurance, because what is unaffected
 * depends on what was asked for.
 */
export function explanationFailureText(
  reason: string,
  scope: 'selection' | 'tree' = 'selection'
): string {
  let lead: string;
  switch (reason) {
    case 'not_configured':
      lead = 'AI-written explanations are not switched on for this deployment, so none is shown.';
      break;
    case 'unauthorized':
      lead = 'The explanation service rejected Everroot’s credentials, so no generated explanation is shown.';
      break;
    case 'rate_limited':
      lead =
        'The explanation service is rate limiting Everroot right now, so no generated explanation is shown.';
      break;
    case 'timeout':
      lead = 'The explanation service did not answer in time, so no generated explanation is shown.';
      break;
    case 'empty_response':
      lead = 'The explanation service returned nothing usable, so no generated text is shown.';
      break;
    default:
      lead = 'The explanation service could not be reached, so no generated explanation is shown.';
  }
  return scope === 'tree'
    ? `${lead} This species’ botanical records are unaffected.`
    : `${lead} The botanical records below are unaffected.`;
}

/* ---------------------------------------------------------------------------
   Flow state
--------------------------------------------------------------------------- */

export type StepId = 'intention' | 'followup' | 'tradition' | 'nakshatra';

export interface FlowAnswers {
  intention: IntentionId | null;
  followUpChoice: string | null;
  followUpNote: string;
  /** Short, predefined starting points the user opted into. */
  followUpPicks: string[];
  tradition: string | null;
  traditionName: string;
  /** Empty string means "any region". */
  region: string;
  nakshatra: string | null;
}

export function emptyAnswers(): FlowAnswers {
  return {
    intention: null,
    followUpChoice: null,
    followUpNote: '',
    followUpPicks: [],
    tradition: null,
    traditionName: '',
    region: '',
    nakshatra: null,
  };
}

/** True when the chosen path can reach a reviewed Nakshatra association. */
export function usesNakshatra(answers: FlowAnswers): boolean {
  if (answers.intention === 'culture') return answers.followUpChoice === 'nakshatra';
  if (answers.intention === 'explore') return answers.tradition === NAKSHATRA_TRADITION;
  return false;
}

/** The ordered step list for the current answers. */
export function pathFor(answers: FlowAnswers): StepId[] {
  const steps: StepId[] = ['intention'];
  if (!answers.intention) return steps;
  if (answers.intention !== 'explore') steps.push('followup');
  if (answers.intention === 'explore') steps.push('tradition');
  if (usesNakshatra(answers)) steps.push('nakshatra');
  return steps;
}

/**
 * The only thing Everroot can query for reviewed records is a
 * tradition + nakshatra pair. Anything else is honestly presented as
 * "nothing on record yet" instead of a fabricated match.
 */
export function discoveryRequest(answers: FlowAnswers): DiscoveryRequestInput | null {
  if (!answers.tradition || !answers.nakshatra) return null;
  return {
    tradition: answers.tradition,
    region: answers.region,
    nakshatra: answers.nakshatra,
  };
}

/* ---------------------------------------------------------------------------
   Storage (all local to the device)
--------------------------------------------------------------------------- */

const FLOW_KEY = 'everroot.flow.v1';
const RECORD_PREFIX = 'everroot.discovery.v1.';
const RECORD_INDEX = 'everroot.discovery.index.v1';
const SAVED_KEY = 'everroot.saved.v1';
const DEDICATION_KEY = 'everroot.dedications.v1';

function store(kind: 'local' | 'session'): Storage | null {
  try {
    const s = kind === 'local' ? window.localStorage : window.sessionStorage;
    const probe = '__everroot_probe__';
    s.setItem(probe, '1');
    s.removeItem(probe);
    return s;
  } catch {
    return null;
  }
}

function readJSON<T>(kind: 'local' | 'session', key: string): T | null {
  const s = store(kind);
  if (!s) return null;
  try {
    const raw = s.getItem(key);
    return raw ? (JSON.parse(raw) as T) : null;
  } catch {
    return null;
  }
}

function writeJSON(kind: 'local' | 'session', key: string, value: unknown): boolean {
  const s = store(kind);
  if (!s) return false;
  try {
    s.setItem(key, JSON.stringify(value));
    return true;
  } catch {
    return false;
  }
}

export function uid(): string {
  try {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
      return crypto.randomUUID();
    }
  } catch {
    /* fall through */
  }
  return `id-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}

/* --- in-progress flow --- */

export interface FlowState {
  answers: FlowAnswers;
  step: StepId;
}

export function loadFlow(): FlowState | null {
  const raw = readJSON<FlowState>('session', FLOW_KEY);
  if (!raw || typeof raw !== 'object' || !raw.answers) return null;
  return { answers: { ...emptyAnswers(), ...raw.answers }, step: raw.step ?? 'intention' };
}

export function saveFlow(state: FlowState): void {
  writeJSON('session', FLOW_KEY, state);
}

export function clearFlow(): void {
  const s = store('session');
  try {
    s?.removeItem(FLOW_KEY);
  } catch {
    /* ignore */
  }
}

/* --- completed discoveries --- */

export interface Dedication {
  text: string;
  visibility: 'private';
  updatedAt: string;
}

export interface DiscoveryRecord {
  id: string;
  createdAt: string;
  answers: FlowAnswers;
  /** Null until the reviewed dataset has been queried (or when there is nothing to query). */
  discovery: DiscoveryResponse | null;
  /** Set when the API call failed — surfaced honestly, never hidden. */
  error: string | null;
}

export function saveRecord(record: DiscoveryRecord): boolean {
  const ok = writeJSON('local', RECORD_PREFIX + record.id, record);
  if (!ok) return false;
  const index = readJSON<string[]>('local', RECORD_INDEX) ?? [];
  const next = [record.id, ...index.filter((id) => id !== record.id)].slice(0, 12);
  writeJSON('local', RECORD_INDEX, next);
  // Prune records that fell out of the index.
  for (const old of index) {
    if (!next.includes(old)) {
      try {
        store('local')?.removeItem(RECORD_PREFIX + old);
      } catch {
        /* ignore */
      }
    }
  }
  return true;
}

export function loadRecord(id: string): DiscoveryRecord | null {
  return readJSON<DiscoveryRecord>('local', RECORD_PREFIX + id);
}

/* --- saved trees (device-local only) --- */

export interface SavedTree {
  key: string;
  label: string;
  scientificName: string;
  recordId: string;
  savedAt: string;
}

export function loadSaved(): SavedTree[] {
  return readJSON<SavedTree[]>('local', SAVED_KEY) ?? [];
}

export function toggleSaved(entry: Omit<SavedTree, 'savedAt'>): SavedTree[] {
  const current = loadSaved();
  const exists = current.some((s) => s.key === entry.key);
  const next = exists
    ? current.filter((s) => s.key !== entry.key)
    : [{ ...entry, savedAt: new Date().toISOString() }, ...current];
  writeJSON('local', SAVED_KEY, next);
  return next;
}

/* --- dedications (device-local preview only) --- */

export function loadDedication(recordId: string): Dedication | null {
  const all = readJSON<Record<string, Dedication>>('local', DEDICATION_KEY) ?? {};
  return all[recordId] ?? null;
}

export function saveDedication(recordId: string, text: string): boolean {
  const all = readJSON<Record<string, Dedication>>('local', DEDICATION_KEY) ?? {};
  all[recordId] = { text, visibility: 'private', updatedAt: new Date().toISOString() };
  return writeJSON('local', DEDICATION_KEY, all);
}

export function removeDedication(recordId: string): void {
  const all = readJSON<Record<string, Dedication>>('local', DEDICATION_KEY) ?? {};
  delete all[recordId];
  writeJSON('local', DEDICATION_KEY, all);
}

export function validURL(value: string): string | null {
  try {
    const url = new URL(value);
    return url.protocol === 'http:' || url.protocol === 'https:' ? url.href : null;
  } catch {
    return null;
  }
}

/* --- User AI Questions & Feedback --- */

export interface QuestionRequest {
  question: string;
  sessionId?: string;
  intention?: string;
  context?: Record<string, string>;
}

export interface QuestionResponse {
  status: string;
  interactionId?: string;
  question: string;
  answer: string;
  promptVersion: string;
  model: string;
  disclaimer: string;
}

export async function askQuestion(input: QuestionRequest): Promise<QuestionResponse> {
  const res = await fetch(apiURL('/api/v1/questions'), {
    method: 'POST',
    headers: { 'content-type': 'application/json', accept: 'application/json' },
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    const errData = (await res.json().catch(() => ({}))) as { error?: string };
    throw new Error(errData.error ?? `AI request failed (${res.status})`);
  }
  return (await res.json()) as QuestionResponse;
}

export interface FeedbackInput {
  interactionId?: string;
  feedbackType?: 'answer' | 'website';
  rating?: number;
  comment?: string;
  correction?: string;
}

export async function submitFeedback(input: FeedbackInput): Promise<void> {
  try {
    await fetch(apiURL('/api/v1/feedback'), {
      method: 'POST',
      headers: { 'content-type': 'application/json', accept: 'application/json' },
      body: JSON.stringify(input),
    });
  } catch {
    // Non-blocking
  }
}
