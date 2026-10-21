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

import { apiSameOriginSetting, apiURLSetting, developmentBuild } from './apiEnv';

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
      'A few words about what this tree is for. What you write is sent to the AI model that draws your suggestions — Everroot does not put it in its records.',
    prompt: 'A quality, a beginning, or something you are working toward',
    placeholder: 'e.g. Patience — for a year of learning to slow down.',
    hint: 'Optional. A few words are enough, and you can change this later.',
    quickPicks: ['A quality I value', 'A milestone or aspiration', 'A connection with nature'],
  },
  remember: {
    question: 'What qualities or memories would you like this tree to represent?',
    intro:
      'You can write a quality, a memory, or a name — or skip this. There is no need to describe anything painful. What you write is sent to the AI model that draws your suggestions; Everroot does not put it in its records.',
    prompt: 'A quality, a memory, or a name',
    placeholder: 'e.g. Quiet steadiness — the way she made room for everyone.',
    hint: 'Optional. Whatever you write is used to draw suggestions and is not put in Everroot’s records.',
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

/**
 * One nakshatra as a chosen naming system names it.
 *
 * `canonical` is always the romanised form the API and the reviewed records are
 * addressed with, so the spelling a visitor picks in their own language is what
 * they are shown while a stable name is what travels on. It is optional because
 * the unqualified list has no second spelling to report alongside its own.
 */
export interface NamedNakshatra extends Nakshatra {
  canonical?: string;
}

/**
 * A language and writing system Everroot can serve nakshatra names in.
 *
 * A visitor usually knows the twenty-seven by the names their own tradition
 * uses. Offering only romanised Sanskrit would quietly make one tradition's
 * spelling the default for everyone, so the choice is asked for first and the
 * list is served in it.
 */
export interface NamingSystem {
  /** Stable code sent back to the API. */
  code: string;
  /** How the system is described in English. */
  name: string;
  /** How speakers name it themselves, so the choice is recognisable at a glance. */
  endonym: string;
  /** The writing system the served names use. */
  script: string;
}

export const NAKSHATRA_TRADITION = 'nakshatra-tree-association';

/**
 * joinAPIBase is the pure part of apiURL, split out so the joining rules can be
 * tested without a bundler. An empty base means "same origin".
 */
export function joinAPIBase(base: string, path: string): string {
  const origin = String(base ?? '').trim().replace(/\/+$/, '');
  const suffix = path.startsWith('/') ? path : `/${path}`;
  return `${origin}${suffix}`;
}

/**
 * What the interface says when the deployment names no API address. It is
 * exported so the tests pin the exact promise made to the operator.
 */
export const API_CONFIGURATION_ERROR =
  'This deployment has no catalogue API address configured, so the request was not sent. Set PUBLIC_API_URL to the backend origin and redeploy. If the API really is served from this same origin, set PUBLIC_API_SAME_ORIGIN=true instead.';

/**
 * apiBase is the configured backend origin, or an empty string when the build
 * had none.
 *
 * `PUBLIC_API_URL` reaches the bundle through SvelteKit's `$env/static/public`
 * (see `apiEnv.ts`), which is inlined at build time and does not depend on
 * Vite's `envPrefix`. It is only needed when the API is served from a different
 * origin than the web app. Setting it requires `WEB_ORIGIN` on the API to name
 * this app's origin, because a cross-origin call is refused without it.
 */
export function apiBase(): string {
  return String(apiURLSetting ?? '').trim().replace(/\/+$/, '');
}

/** True when this build names a backend origin. */
export function isAPIConfigured(): boolean {
  return apiBase() !== '';
}

/**
 * Whether a same-origin request is acceptable.
 *
 * Under `vite dev` the dev server proxies `/api`, so it always is. A production
 * build has to say so explicitly with `PUBLIC_API_SAME_ORIGIN=true`, because
 * otherwise a missing `PUBLIC_API_URL` quietly turns every catalogue call into a
 * request to the web app's own origin — which serves no API and answers with an
 * HTML 404. That silent fallback is exactly what made the original
 * misconfiguration so hard to see, so it is no longer the default.
 *
 * Pure so both deployment models can be tested without a bundler.
 */
export function sameOriginAllowed(
  development: boolean,
  sameOriginSetting: string | undefined
): boolean {
  return development || String(sameOriginSetting ?? '').trim().toLowerCase() === 'true';
}

/** The reason no catalogue request may be made, or null when one may. */
export function missingAPIConfiguration(): string | null {
  if (isAPIConfigured() || sameOriginAllowed(developmentBuild, apiSameOriginSetting)) return null;
  return API_CONFIGURATION_ERROR;
}

/**
 * apiURL is the single helper every API call goes through. The base is a
 * parameter so the joining rules can be tested against a configured origin
 * without a bundler; every caller uses the build-time default.
 */
export function apiURL(path: string, base: string = apiBase()): string {
  return joinAPIBase(base, path);
}

/**
 * The distinctions the interface has to make. A 404 means the origin that
 * answered serves no API at this path (a routing or configuration problem); a
 * 503 means the API answered but cannot serve its reviewed records; a transport
 * failure means the API was never reached. None of them is "no records", so
 * none of them may be presented as one.
 */
export type APIErrorKind =
  | 'config'
  | 'not_found'
  | 'unavailable'
  | 'server'
  | 'request'
  | 'network';

export class APIError extends Error {
  readonly kind: APIErrorKind;
  readonly status: number | null;

  constructor(kind: APIErrorKind, message: string, status: number | null = null) {
    super(message);
    this.name = 'APIError';
    this.kind = kind;
    this.status = status;
  }
}

/** Map an HTTP status to the one distinction the interface needs. */
export function classifyStatus(status: number): APIErrorKind {
  if (status === 404) return 'not_found';
  if (status === 503) return 'unavailable';
  if (status >= 500) return 'server';
  return 'request';
}

/**
 * Honest copy for an API failure.
 *
 * A 404 in a build that named no backend origin is a configuration problem: the
 * request could only have gone to the web app's own origin, which serves no
 * API, and the copy says so and names PUBLIC_API_URL. Everything else is
 * reported as itself, so a visitor is never told the catalogue is empty. The
 * `configured` flag is a parameter so every branch is testable without a
 * build-time value.
 */
export function apiErrorMessage(error: unknown, configured: boolean = isAPIConfigured()): string {
  const kind = error instanceof APIError ? error.kind : 'network';
  const status = error instanceof APIError ? error.status : null;

  if (!configured && kind === 'not_found') {
    return 'This build has no catalogue API address configured, so the request went to this site’s own origin and was not found. Set PUBLIC_API_URL to the backend origin and redeploy.';
  }

  switch (kind) {
    case 'config':
      return API_CONFIGURATION_ERROR;
    case 'not_found':
      return 'The catalogue API was not found at the configured address (404). Check that PUBLIC_API_URL names the backend origin.';
    case 'unavailable':
      return 'The reviewed catalogue is unavailable right now (503). The API answered, but its reviewed records cannot be served yet — please try again shortly.';
    case 'server':
      return `The catalogue API reported a server error (${status ?? 500}). Please try again shortly.`;
    case 'request':
      return `The catalogue API rejected the request (${status ?? 400}).`;
    default:
      return 'The catalogue API could not be reached. Check your connection and try again.';
  }
}

async function getJSON<T>(path: string, base: string = apiBase()): Promise<T> {
  const origin = String(base ?? '').trim().replace(/\/+$/, '');
  if (!origin) {
    // A production build that names no API address must not quietly send the
    // request to this site's own origin. That is a configuration error, and the
    // caller is told so — with nothing sent — instead of being shown an HTML 404
    // that looks like a missing catalogue.
    const misconfigured = missingAPIConfiguration();
    if (misconfigured) throw new APIError('config', misconfigured);
  }
  const url = apiURL(path, origin);
  let res: Response;
  try {
    res = await fetch(url, { headers: { accept: 'application/json' } });
  } catch {
    throw new APIError('network', `The request to ${url} failed before a response was received.`);
  }
  if (!res.ok) {
    throw new APIError(classifyStatus(res.status), `Request failed (${res.status})`, res.status);
  }
  return (await res.json()) as T;
}

export function fetchTraditions(base: string = apiBase()): Promise<Tradition[]> {
  return getJSON<Tradition[]>('/api/v1/traditions', base);
}

export function fetchNakshatras(base: string = apiBase()): Promise<Nakshatra[]> {
  return getJSON<Nakshatra[]>('/api/v1/nakshatras', base);
}

export function fetchNamingSystems(base: string = apiBase()): Promise<NamingSystem[]> {
  return getJSON<NamingSystem[]>('/api/v1/naming-systems', base);
}

/**
 * The twenty-seven as a chosen naming system names them.
 *
 * An unrecognised code still returns all twenty-seven — the sequence is the
 * same everywhere and only the spelling differs — so a caller that wants to
 * know whether it got what it asked for compares the code against
 * fetchNamingSystems rather than treating an unknown code as a failure.
 */
export function fetchNakshatrasIn(code: string, base: string = apiBase()): Promise<NamedNakshatra[]> {
  return getJSON<NamedNakshatra[]>(
    `/api/v1/nakshatras/${encodeURIComponent(code)}`,
    base
  );
}

export function fetchRegions(traditionSlug: string, base: string = apiBase()): Promise<string[]> {
  return getJSON<string[]>(
    `/api/v1/traditions/${encodeURIComponent(traditionSlug)}/regions`,
    base
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
   AI-first discovery (research, then recommend)

   This is a different endpoint from runDiscovery above. runDiscovery asks the
   publication gate whether a REVIEWED cultural association exists for a
   selection; this one asks the model to research the question and recommend,
   attributing every claim to the sources it retrieved. The two are deliberately
   not aliases, and neither is a fallback for the other.
--------------------------------------------------------------------------- */

/** What a claim's support actually is. A closed vocabulary so the interface can
 * render it consistently, and so a reply can never introduce a new one. */
export type EvidenceLevel = 'sourced' | 'uncertain' | 'interpretation' | 'user_defined' | 'none';

/** One statement about a candidate, with the evidence behind it. */
export interface AIClaim {
  text: string;
  evidence: EvidenceLevel;
  sourceIds: string[];
}

/** What a taxonomic record established about a candidate's scientific name.
 * Written by the server, never by the model. */
export interface CandidateIdentity {
  status: 'confirmed' | 'not_found' | 'unverified' | 'not_attempted';
  acceptedName?: string;
  family?: string;
  source?: string;
}

/** A candidate's overall assessment. The level is the model's honest read of
 * the retrieved evidence — it is NOT an independently measured score, which is
 * why the definition and the assessedBy attribution travel with it. */
export interface CandidateConfidence {
  level: 'high' | 'moderate' | 'low' | 'insufficient';
  definition: string;
  assessedBy: string;
}

/** One tree the model offered. */
export interface AICandidate {
  name: string;
  scientificName: string;
  whyItMatches: string;
  botanicalDetail: string;
  matureSize: string;
  growingRequirements?: string[];
  limitations?: string[];
  culturalSignificance: AIClaim;
  ecologicalFit: AIClaim;
  identity: CandidateIdentity;
  confidence: CandidateConfidence;
  sourceIds: string[];
}

/** One retrieved finding. The URL is the real address the server resolved, not
 * something the model wrote. */
export interface ResearchSource {
  id: string;
  title: string;
  url: string;
  publisher?: string;
  kind: 'web_page' | 'encyclopedia' | 'biodiversity_record' | 'scholarly';
  snippet?: string;
  retrievedAt: string;
}

/**
 * How the answer was produced.
 *
 * `researched`     — sources were retrieved and the answer is grounded in them.
 * `partial`        — some sources were retrieved, but at least one backend failed.
 * `unavailable`    — research service temporarily unavailable. The answer is provisional.
 * `no_sources`     — research succeeded but no authoritative sources matched these specific terms.
 * `not_configured` — research provider is not configured in this server environment.
 */
export type ResearchStatus =
  | 'researched'
  | 'partial'
  | 'unavailable'
  | 'no_sources'
  | 'not_configured';

export interface AIDiscoveryResponse {
  /** `no_candidates` is a genuine answer: the model found nothing it would
   * stand behind. It is never an error. */
  status: 'ok' | 'no_candidates';
  candidates: AICandidate[];
  sources: ResearchSource[];
  researchStatus: ResearchStatus;
  researchSummary: string;
  unverifiedAreas?: string[];
  followUpQuestion?: string;
  confidenceDefinitions: Record<string, string>;
  promptVersion: string;
  model: string;
  disclaimer: string;
  /** Present only when the answer was saved against the caller's session. */
  runId?: string;
  /** False means the answer was NOT kept. The interface must say so rather
   * than leaving a visitor to assume their discovery was stored. */
  saved: boolean;
}

export interface AIDiscoveryRequest {
  /** Either one of the interface's intention keys, or the visitor's own words.
   * An unfamiliar question is not refused; it reaches the model. */
  intention: string;
  description?: string;
  tradition?: string;
  region?: string;
  nakshatra?: string;
  constraints?: string[];
  limit?: number;
}

/**
 * Runs the AI-first discovery pipeline.
 *
 * A provider failure throws, and that is deliberate: an inference outage must
 * reach the visitor as an error, never as a list with nothing in it, because
 * the two look identical as an empty array and mean very different things.
 *
 * A RESEARCH failure does not throw. It arrives as researchStatus
 * 'unavailable' with candidates still present, which is the pipeline's way of
 * saying "this is provisional and nobody looked anything up".
 */
export async function runAIDiscovery(
  input: AIDiscoveryRequest,
  token?: string,
  base: string = apiBase()
): Promise<AIDiscoveryResponse> {
  const headers: Record<string, string> = {
    'content-type': 'application/json',
    accept: 'application/json',
  };
  if (token) headers['authorization'] = `Bearer ${token}`;

  let res: Response;
  try {
    res = await fetch(apiURL('/api/v1/discovery', base), {
      method: 'POST',
      headers,
      body: JSON.stringify(input),
    });
  } catch {
    throw new Error('The discovery service could not be reached from this browser.');
  }

  if (!res.ok) {
    const body = (await res.json().catch(() => ({}))) as { error?: string };
    // The server's message is written for a visitor and carries no internals,
    // so it is preferred over a generic one; the status-derived message is the
    // fallback when the body could not be read.
    throw new Error(body.error || apiErrorMessage({ kind: classifyStatus(res.status) }, isAPIConfigured()));
  }

  const data = (await res.json()) as Partial<AIDiscoveryResponse>;
  return {
    status: data.status === 'no_candidates' ? 'no_candidates' : 'ok',
    candidates: Array.isArray(data.candidates) ? data.candidates : [],
    sources: Array.isArray(data.sources) ? data.sources : [],
    researchStatus: data.researchStatus ?? 'unavailable',
    researchSummary: data.researchSummary ?? '',
    unverifiedAreas: Array.isArray(data.unverifiedAreas) ? data.unverifiedAreas : undefined,
    followUpQuestion: data.followUpQuestion,
    confidenceDefinitions: data.confidenceDefinitions ?? {},
    promptVersion: data.promptVersion ?? '',
    model: data.model ?? '',
    disclaimer: data.disclaimer ?? '',
    runId: data.runId,
    saved: data.saved === true,
  };
}

/** The visitor's own sources for a candidate, in citation order.
 *
 * A citation that does not resolve to a retrieved source is skipped rather
 * than rendered as a broken entry. The server has already rejected an ID the
 * model invented, so this is a second line of defence rather than the first. */
export function sourcesFor(response: AIDiscoveryResponse, ids: string[]): ResearchSource[] {
  const byId = new Map(response.sources.map((s) => [s.id, s]));
  return ids.map((id) => byId.get(id)).filter((s): s is ResearchSource => Boolean(s));
}

/** Every source the answer drew on, in first-cited order. This is what the
 * results page renders as its "sources consulted" list. */
export function distinctSources(response: AIDiscoveryResponse): ResearchSource[] {
  const seen = new Set<string>();
  const out: ResearchSource[] = [];
  for (const candidate of response.candidates) {
    for (const source of sourcesFor(response, candidate.sourceIds ?? [])) {
      if (seen.has(source.id)) continue;
      seen.add(source.id);
      out.push(source);
    }
  }
  return out;
}

/**
 * Reshapes one researched candidate into the card the shortlist already knows.
 *
 * It is a projection, not a translation of meaning: every field it sets is one
 * the candidate actually carries, and anything the two shapes do not share is
 * left unset rather than filled in. The point of the adapter is that the tree
 * a visitor reads about and the tree the provenance panel describes are the
 * same tree — a shortlist drawn from one endpoint and provenance drawn from
 * another would let a sourced claim sit underneath a card it never supported.
 */
export function candidateToSuggestion(
  candidate: AICandidate,
  allSources: ResearchSource[] = []
): RecommendationSuggestion {
  const sources =
    candidate.sourceIds?.length && allSources.length
      ? allSources.filter((s) => candidate.sourceIds.includes(s.id))
      : [];
  const suggestion: RecommendationSuggestion = {
    name: candidate.name,
    scientificName: candidate.scientificName ?? '',
    whyItMatches: candidate.whyItMatches,
    botanicalDetail: candidate.botanicalDetail ?? '',
    matureSize: candidate.matureSize ?? '',
    growingRequirements: candidate.growingRequirements ?? [],
    limitations: candidate.limitations ?? [],
  };
  if (sources.length) suggestion.sources = sources;
  if (candidate.culturalSignificance) suggestion.culturalSignificance = candidate.culturalSignificance;
  if (candidate.ecologicalFit) suggestion.ecologicalFit = candidate.ecologicalFit;
  if (candidate.identity) suggestion.identity = candidate.identity;
  if (candidate.confidence) suggestion.confidence = candidate.confidence;
  return suggestion;
}

/** How the interface should describe one evidence level in plain words.
 * Kept here rather than in the component so the wording cannot drift between
 * the results page and any other surface that shows a claim. */
export const EVIDENCE_LABELS: Record<EvidenceLevel, string> = {
  sourced: 'Backed by a source',
  uncertain: 'Only partly supported',
  interpretation: 'An interpretation, not a documented fact',
  user_defined: 'Your own meaning',
  none: 'No evidence found',
};

/** Whether a claim should read as settled. Only `sourced` does; everything
 * else is shown with its caveat, because presenting an interpretation as a
 * fact is the exact failure this pipeline exists to prevent. */
export function isClaimSourced(claim: AIClaim | undefined | null): boolean {
  return claim?.evidence === 'sourced' && (claim.sourceIds?.length ?? 0) > 0;
}

/* ---------------------------------------------------------------------------
   Sessions and saved discoveries

   A session is optional: a discovery never requires an account, and Everroot
   never asks for an email, a name or a date of birth. The token is returned
   once and is not recoverable afterwards — only its hash is stored server-side.
--------------------------------------------------------------------------- */

const SESSION_TOKEN_KEY = 'everroot.session.token';

/** Reads the stored session token, if any. Never throws: a storage failure
 * simply means the visitor is treated as having no session. */
export function loadSessionToken(): string | null {
  try {
    return store('local')?.getItem(SESSION_TOKEN_KEY) ?? null;
  } catch {
    return null;
  }
}

export function saveSessionToken(token: string): void {
  try {
    store('local')?.setItem(SESSION_TOKEN_KEY, token);
  } catch {
    /* a full or blocked storage must not break the flow */
  }
}

export function clearSessionToken(): void {
  try {
    store('local')?.removeItem(SESSION_TOKEN_KEY);
  } catch {
    /* ignore */
  }
}

/** Obtains an anonymous session token, or returns null when none could be
 * issued. It is never an error to be without one — a discovery works either
 * way; it just cannot be saved. */
export async function createSession(base: string = apiBase()): Promise<string | null> {
  let res: Response;
  try {
    res = await fetch(apiURL('/api/v1/session', base), {
      method: 'POST',
      headers: { 'content-type': 'application/json', accept: 'application/json' },
      body: '{}',
    });
  } catch {
    return null;
  }
  if (!res.ok) return null;
  const data = (await res.json().catch(() => ({}))) as { token?: string };
  return typeof data.token === 'string' && data.token ? data.token : null;
}

/** The caller's saved discoveries, newest first. Returns an empty list rather
 * than throwing when there is no session or the store is unavailable, because
 * a visitor with no saved discoveries and a visitor whose history cannot be
 * read both have nothing to show. */
export async function listSavedDiscoveries(base: string = apiBase()): Promise<SavedDiscoverySummary[]> {
  const token = loadSessionToken();
  if (!token) return [];
  let res: Response;
  try {
    res = await fetch(apiURL('/api/v1/discovery', base), {
      headers: { accept: 'application/json', authorization: `Bearer ${token}` },
    });
  } catch {
    return [];
  }
  if (!res.ok) return [];
  const data = (await res.json().catch(() => ({}))) as { discoveries?: SavedDiscoverySummary[] };
  return Array.isArray(data.discoveries) ? data.discoveries : [];
}

export interface SavedDiscoverySummary {
  id: string;
  intention: string;
  researchStatus: ResearchStatus;
  status: 'running' | 'complete' | 'failed';
  createdAt: string;
  sourceCount: number;
}

/** Deletes one of the caller's own discoveries. Returns false when it could
 * not be deleted, including when it was not theirs — the server deliberately
 * reports those the same way, so an id cannot be probed. */
export async function deleteSavedDiscovery(id: string, base: string = apiBase()): Promise<boolean> {
  const token = loadSessionToken();
  if (!token) return false;
  try {
    const res = await fetch(apiURL(`/api/v1/discovery/${encodeURIComponent(id)}`, base), {
      method: 'DELETE',
      headers: { authorization: `Bearer ${token}` },
    });
    return res.status === 204;
  } catch {
    return false;
  }
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
    evidence: data.evidence ?? emptyEvidence(),
    trees: Array.isArray(data.trees) ? data.trees : [],
    explanation: data.explanation ?? { status: 'unavailable', reason: 'provider_error', text: '', model: '' },
  };
}

/* ---------------------------------------------------------------------------
   AI suggestions

   A separate call from /api/v1/discoveries on purpose. That endpoint answers
   what Everroot's publication gate has approved and can never say yes without
   a reviewer; this one asks the configured model what else might fit these
   answers. Neither borrows the other's authority: a suggestion is machine
   written wherever it is shown, and it never carries a cultural claim.
--------------------------------------------------------------------------- */

export interface RecommendationSuggestion {
  name: string;
  scientificName: string;
  whyItMatches: string;
  botanicalDetail: string;
  /** Typical mature height, or the literal text "not sure". */
  matureSize?: string;
  growingRequirements: string[];
  limitations: string[];
  /**
   * The model's one plain sentence restating the gate verdict it was handed.
   * Present only when evidence.culturalAssociation is "no_reviewed_match", and
   * it describes the record rather than the tree — so it never replaces the
   * server's own verdict, which travels beside it as evidence.
   */
  culturalNote?: string;
  sources?: ResearchSource[];
  culturalSignificance?: AIClaim;
  ecologicalFit?: AIClaim;
  identity?: CandidateIdentity;
  confidence?: CandidateConfidence;
}

export interface RecommendationsResponse {
  /** "ok" when the model offered something, "no_suggestions" when it honestly had none. */
  status: 'ok' | 'no_suggestions' | string;
  suggestions: RecommendationSuggestion[];
  /** The publication gate's verdict about the selection, never the model's opinion. */
  evidence: EvidenceStatus;
  promptVersion: string;
  model: string;
  disclaimer: string;
  /**
   * Omitted by the API when there is none. Present only when one missing fact
   * would materially change the suggestions — and it never stands in for an
   * answer, because the suggestions arrive with it.
   */
  followUpQuestion?: string;
}

export interface RecommendationsRequest {
  intention: IntentionId;
  answer: string;
  choice: string;
  picks: string[];
  tradition: string;
  region: string;
  nakshatra: string;
  constraints: string[];
  limit: number;
}

/** Empty evidence, for a reply that arrived without it. */
function emptyEvidence(): EvidenceStatus {
  return {
    culturalAssociation: 'not_queryable',
    botanicalIdentity: 'unavailable',
    habitat: 'no_reviewed_record',
    ecologicalRole: 'no_reviewed_record',
    documentedStories: 'no_reviewed_record',
    personalMeaning: 'user_owned',
  };
}

/**
 * The visitor's selections and their own words, shaped for the endpoint.
 *
 * Nothing here is required by the server — an intention is the only field it
 * insists on — so a path that never reached the cultural steps still produces a
 * request, and a cultural selection arrives as context rather than as a claim.
 */
export function recommendationsRequest(answers: FlowAnswers): RecommendationsRequest {
  return {
    // Every step of the flow leaves an intention behind. A record written
    // before the flow existed is treated as open curiosity rather than as
    // something personal the visitor never chose to share.
    intention: answers.intention ?? 'explore',
    answer: answers.followUpNote.trim(),
    choice:
      followUpOptionLabel(answers.intention, answers.followUpChoice) ?? answers.followUpChoice ?? '',
    picks: [...answers.followUpPicks],
    tradition: answers.tradition ?? '',
    region: answers.region ?? '',
    nakshatra: answers.nakshatra ?? '',
    // Practical requirements, when the caller has any to state. The flow keeps
    // them in the visitor's own words instead, which is what `answer` carries.
    constraints: [],
    limit: 3,
  };
}

export function aiDiscoveryRequest(answers: FlowAnswers): AIDiscoveryRequest {
  const label =
    followUpOptionLabel(answers.intention, answers.followUpChoice) ?? answers.followUpChoice ?? '';
  const parts: string[] = [];

  if (answers.customIntention?.trim()) parts.push(answers.customIntention.trim());
  if (answers.followUpNote?.trim()) parts.push(answers.followUpNote.trim());
  if (label.trim()) parts.push(label.trim());
  for (const pick of answers.followUpPicks ?? []) {
    if (pick.trim()) parts.push(pick.trim());
  }
  if (answers.motivations?.length) {
    parts.push(`Motivations: ${answers.motivations.join(', ')}`);
  }
  if (answers.clarification?.trim()) {
    parts.push(`Cultural context: ${answers.clarification.trim()}`);
  }

  // Determine tradition dynamically from user input or chosen context
  let tradition = answers.tradition ?? '';
  if (answers.customCulture?.trim()) {
    tradition = answers.customCulture.trim();
  }

  // Check astrology system
  const text = `${answers.customIntention ?? ''} ${answers.followUpNote ?? ''} ${answers.customCulture ?? ''}`;
  const astro = detectAstrologyContext(text, answers.followUpChoice);
  const astroSystem =
    answers.astrologySystem ?? (astro.system || (astro.isAstrology ? 'western' : undefined));

  if (astroSystem === 'western') {
    if (!tradition) tradition = 'Western zodiac (default)';
    parts.push('Astrological tradition: Western zodiac (default)');
  } else if (astroSystem === 'vedic') {
    if (!tradition) tradition = 'Indian / Vedic astrology (Nakshatras)';
    parts.push('Astrological tradition: Indian / Vedic astrology');
  } else if (astroSystem === 'tamil') {
    if (!tradition) tradition = 'Tamil astrological tradition';
    parts.push('Astrological tradition: Tamil astrological tradition');
  } else if (astroSystem === 'other' && answers.clarification) {
    if (!tradition) tradition = answers.clarification;
  }

  const intention =
    answers.intention ??
    (answers.customIntention?.trim() ? answers.customIntention.trim() : 'explore');

  return {
    intention,
    description: parts.filter(Boolean).join('. '),
    tradition,
    region: answers.region ?? '',
    nakshatra: answers.nakshatra ?? '',
    constraints: [],
    limit: 3,
  };
}

export async function runRecommendations(
  input: RecommendationsRequest
): Promise<RecommendationsResponse> {
  let res: Response;
  try {
    res = await fetch(apiURL('/api/v1/recommendations'), {
      method: 'POST',
      headers: { 'content-type': 'application/json', accept: 'application/json' },
      body: JSON.stringify(input),
    });
  } catch {
    throw new Error('The suggestion service could not be reached from this browser.');
  }
  if (!res.ok) {
    // A failure is always an exception here: the page must never read a broken
    // service as "Everroot has nothing to suggest".
    throw new Error(
      res.status === 429
        ? 'The AI service is busy right now. Please try again in a moment.'
        : res.status >= 500
          ? 'Tree suggestions are unavailable right now. Please try again.'
          : `The suggestion request was rejected (${res.status}).`
    );
  }
  const data = (await res.json()) as Partial<RecommendationsResponse>;
  const followUpQuestion = typeof data.followUpQuestion === 'string' ? data.followUpQuestion.trim() : '';
  return {
    status: data.status ?? 'ok',
    suggestions: Array.isArray(data.suggestions) ? data.suggestions : [],
    evidence: data.evidence ?? emptyEvidence(),
    promptVersion: data.promptVersion ?? '',
    model: data.model ?? '',
    disclaimer: data.disclaimer ?? '',
    // Trimmed here so a model that padded it with whitespace cannot render a
    // blank question on the results page.
    ...(followUpQuestion ? { followUpQuestion } : {}),
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

export type StepId =
  | 'intention'
  | 'clarification'
  | 'followup'
  | 'tradition'
  | 'culture'
  | 'nakshatra';

export interface FlowAnswers {
  intention: IntentionId | null;
  /** Free-text description of why the user wants a tree */
  customIntention?: string;
  /** Multiple selected motivations, e.g. 'culture', 'astrology', 'ecology', 'memorial', etc. */
  motivations: string[];
  followUpChoice: string | null;
  followUpNote: string;
  /** Short, predefined starting points the user opted into. */
  followUpPicks: string[];
  tradition: string | null;
  traditionName: string;
  /** Specific culture or tradition free-text or choice */
  customCulture?: string;
  /** Astrological system when relevant */
  astrologySystem?: 'western' | 'vedic' | 'tamil' | 'other' | 'explore' | 'none';
  /** Clarification response if asked */
  clarification?: string;
  /** Empty string means "any region". */
  region: string;
  /**
   * The naming system the nakshatra names were shown in. Empty means the
   * visitor expressed no preference, which is not the same as choosing the
   * romanised one — nothing is inferred from their other answers.
   */
  naming: string;
  /**
   * The nakshatra as the API and the reviewed records know it, so a choice
   * made in any naming system still reaches the same record.
   */
  nakshatra: string | null;
  /** The nakshatra as it was shown to the visitor, so the result page can
   *  repeat it back in their own naming system rather than in a transliteration. */
  nakshatraName: string;
}

export function emptyAnswers(): FlowAnswers {
  return {
    intention: null,
    customIntention: '',
    motivations: [],
    followUpChoice: null,
    followUpNote: '',
    followUpPicks: [],
    tradition: null,
    traditionName: '',
    customCulture: '',
    astrologySystem: undefined,
    clarification: '',
    region: '',
    naming: '',
    nakshatra: null,
    nakshatraName: '',
  };
}

export interface AstrologyContext {
  isAstrology: boolean;
  system?: 'western' | 'vedic' | 'tamil' | 'other';
  explicitTerm?: string;
  isAmbiguous: boolean;
}

export function detectAstrologyContext(text: string, choice?: string | null): AstrologyContext {
  const combined = `${text} ${choice ?? ''}`.toLowerCase();

  const tamilTerms = [
    'punarpoosam',
    'tamil',
    'karthigai',
    'avittam',
    'chithirai',
    'magam',
    'visakam',
    'ayilyam',
    'thiruvathirai',
    'moolam',
    'uthiradam',
    'hastham',
    'pooradam',
    'sadayam',
  ];
  for (const term of tamilTerms) {
    if (combined.includes(term)) {
      return { isAstrology: true, system: 'tamil', explicitTerm: term, isAmbiguous: false };
    }
  }

  const vedicTerms = [
    'nakshatra',
    'vedic',
    'jyotish',
    'rashi',
    'ashwini',
    'bharani',
    'krittika',
    'mriga',
    'ardra',
    'pushya',
    'ashlesha',
    'magha',
    'purvaphalguni',
    'uttaraphalguni',
    'hasta',
    'chitra',
    'swati',
    'vishakha',
    'anuradha',
    'jyeshtha',
    'mula',
    'purvashadha',
    'uttarashadha',
    'shravana',
    'dhanishta',
    'shatabhisha',
    'purvabhadrapada',
    'uttarabhadrapada',
    'revati',
  ];
  for (const term of vedicTerms) {
    if (combined.includes(term)) {
      return { isAstrology: true, system: 'vedic', explicitTerm: term, isAmbiguous: false };
    }
  }

  const westernTerms = [
    'western',
    'tropical',
    'aries',
    'taurus',
    'gemini',
    'cancer',
    'leo',
    'virgo',
    'libra',
    'scorpio',
    'sagittarius',
    'capricorn',
    'aquarius',
    'pisces',
  ];
  for (const term of westernTerms) {
    if (combined.includes(term)) {
      return { isAstrology: true, system: 'western', explicitTerm: term, isAmbiguous: false };
    }
  }

  const generalTerms = [
    'astrology',
    'astrological',
    'zodiac',
    'horoscope',
    'star sign',
    'sun sign',
    'moon sign',
    'birth sign',
    'birth star',
  ];
  for (const term of generalTerms) {
    if (combined.includes(term)) {
      return { isAstrology: true, explicitTerm: term, isAmbiguous: true };
    }
  }

  if (choice === 'nakshatra') {
    return { isAstrology: true, system: 'vedic', explicitTerm: 'nakshatra', isAmbiguous: false };
  }

  return { isAstrology: false, isAmbiguous: false };
}

export function needsAstrologyClarification(answers: FlowAnswers): boolean {
  if (answers.astrologySystem) return false;
  const motivations = answers.motivations ?? [];
  const text = `${answers.customIntention ?? ''} ${answers.followUpNote ?? ''} ${answers.customCulture ?? ''} ${answers.traditionName ?? ''}`;
  const detected = detectAstrologyContext(text, answers.followUpChoice);
  if (motivations.includes('astrology') && !detected.system) return true;
  return detected.isAstrology && detected.isAmbiguous;
}

/**
 * Answers restored from storage, with the fields added since the record was
 * written filled in.
 */
export function normaliseAnswers(raw: Partial<FlowAnswers> | null | undefined): FlowAnswers {
  const base = emptyAnswers();
  if (!raw || typeof raw !== 'object') return base;
  return {
    ...base,
    ...raw,
    motivations: Array.isArray(raw.motivations) ? raw.motivations : base.motivations,
    customIntention: typeof raw.customIntention === 'string' ? raw.customIntention : '',
    customCulture: typeof raw.customCulture === 'string' ? raw.customCulture : '',
    clarification: typeof raw.clarification === 'string' ? raw.clarification : '',
    followUpPicks: Array.isArray(raw.followUpPicks) ? raw.followUpPicks : base.followUpPicks,
    naming: typeof raw.naming === 'string' ? raw.naming : '',
    nakshatraName:
      typeof raw.nakshatraName === 'string'
        ? raw.nakshatraName
        : typeof raw.nakshatra === 'string'
          ? raw.nakshatra
          : '',
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
  const hasText = Boolean(answers.customIntention?.trim() || answers.followUpNote?.trim());
  if (!answers.intention && !hasText) return steps;

  if (needsAstrologyClarification(answers)) {
    steps.push('clarification');
    return steps;
  }

  if (answers.intention === 'explore' && !answers.tradition) {
    steps.push('tradition');
  } else if (answers.intention && answers.intention !== 'explore') {
    steps.push('followup');
  }

  if (usesNakshatra(answers)) {
    steps.push('nakshatra');
  }
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
  return { answers: normaliseAnswers(raw.answers), step: raw.step ?? 'intention' };
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
  const record = readJSON<DiscoveryRecord>('local', RECORD_PREFIX + id);
  if (!record) return null;
  // A record written before the naming system was asked for has no `naming` on
  // it. Normalising keeps that result readable instead of handing a template
  // a field that was never stored.
  return { ...record, answers: normaliseAnswers(record.answers) };
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
