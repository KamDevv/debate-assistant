import { useEffect, useMemo, useRef, useState } from 'react';
import SpeechCapture from './SpeechCapture';
import {
  Activity, BookOpen, BrainCircuit, CircleAlert, FileSearch, Gavel,
  History, MessageSquareText, Plus, Radio, Send, ShieldAlert, Sparkles,
  ExternalLink, Search, CheckCircle2, Scale, Lightbulb, ListChecks
} from 'lucide-react';

type Debate = { id: string; title: string; topic: string; created_at?: string };
type Message = { id?: string; speaker: 'you' | 'opponent'; content: string; created_at?: string };
type Source = { id: string; title: string; url: string; domain?: string; snippet: string; published_at?: string; score?: number };
type Finding = {
  category: string; severity: string; title: string; explanation: string;
  suggested_response?: string; question?: string; confidence: string; source_ids?: string[];
};
type Result = {
  summary: string; opponent_findings: Finding[]; your_findings: Finding[];
  suggestions: string[]; evidence_needed: string[]; sources: Source[];
  research_used: boolean; research_warning?: string; disclaimer: string;
};
type Argument = { title: string; reasoning: string; evidence?: string; source_ids?: string[] };
type Fact = { claim: string; assessment: string; context: string; confidence: string; source_ids?: string[] };
type Rebuttal = { challenge: string; response: string; caveat?: string; source_ids?: string[] };
type ResearchReport = {
  id?: string; topic: string; position?: string; generated_at: string; overview: string;
  arguments_for: Argument[]; arguments_against: Argument[]; key_facts: Fact[];
  counterarguments_to_expect: string[]; risks_to_your_position: string[];
  questions_to_ask: string[]; caveats: string[]; sources: Source[]; disclaimer: string;
  devils_advocate?: string; opening_statement?: string; closing_statement?: string;
  rebuttal_bank?: Rebuttal[]; evidence_gaps?: string[]; debate_strategy?: string[];
};
type ResearchSummary = { id: string; topic: string; position?: string; created_at: string };
type LedgerEvidence = {
  id: string; claim_id: string; title: string; url: string; domain?: string;
  snippet: string; published_at?: string; relationship: string; explanation: string; confidence: string;
};
type LedgerClaim = {
  id: string; debate_id: string; speaker: 'you' | 'opponent'; text: string;
  claim_type: string; assessment: string; confidence: string; explanation: string;
  message_ids: string[]; first_seen_at: string; last_seen_at: string; evidence: LedgerEvidence[];
};
type LedgerContradiction = {
  id: string; debate_id: string; claim_a_id: string; claim_b_id: string;
  explanation: string; confidence: string; created_at: string;
};
type LedgerReport = {
  debate_id: string; claims: LedgerClaim[]; contradictions: LedgerContradiction[];
  message_count: number; updated_at?: string; warnings: string[];
};
type Health = { gemini_configured: boolean; gemini_model: string; web_research_configured: boolean };

const API = import.meta.env.VITE_API_URL || 'http://localhost:8080';

export default function App() {
  const [debates, setDebates] = useState<Debate[]>([]);
  const [active, setActive] = useState<Debate | null>(null);
  const [messages, setMessages] = useState<Message[]>([]);
  const [speaker, setSpeaker] = useState<'you' | 'opponent'>('opponent');
  const [draft, setDraft] = useState('');
  const [result, setResult] = useState<Result | null>(null);
  const [topic, setTopic] = useState('');
  const [title, setTitle] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [tab, setTab] = useState<'live' | 'research' | 'history' | 'ledger'>('live');
  const [health, setHealth] = useState<Health | null>(null);
  const [researchTopic, setResearchTopic] = useState('');
  const [researchPosition, setResearchPosition] = useState('');
  const [researchReport, setResearchReport] = useState<ResearchReport | null>(null);
  const [researchHistory, setResearchHistory] = useState<ResearchSummary[]>([]);
  const [researchBusy, setResearchBusy] = useState(false);
  const [ledger, setLedger] = useState<LedgerReport | null>(null);
  const [ledgerBusy, setLedgerBusy] = useState(false);
  const [autoAnalyse, setAutoAnalyse] = useState(false);
  const [liveAnalysisStatus, setLiveAnalysisStatus] = useState('Automatic analysis is off');
  const [lastAnalysisAt, setLastAnalysisAt] = useState('');
  const activeRef = useRef<Debate | null>(null);
  const messagesRef = useRef<Message[]>([]);
  const autoAnalyseRef = useRef(false);
  const speechBufferRef = useRef<{ speaker: 'you' | 'opponent'; content: string } | null>(null);
  const speechFlushTimerRef = useRef<number | undefined>(undefined);
  const liveAnalysisTimerRef = useRef<number | undefined>(undefined);
  const lastLiveAnalysisAtRef = useRef(0);
  const analysisBusyRef = useRef(false);
  const messageQueueRef = useRef<Promise<void>>(Promise.resolve());

  async function request<T>(path: string, init?: RequestInit): Promise<T> {
    const response = await fetch(API + path, {
      headers: { 'Content-Type': 'application/json' },
      ...init,
    });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(data.error || `Request failed (${response.status})`);
    return data as T;
  }

  async function refresh() {
    try { setDebates(await request<Debate[]>('/api/debates')); }
    catch (e) { setError(String(e)); }
  }

  async function refreshResearchHistory() {
    try { setResearchHistory(await request<ResearchSummary[]>('/api/research')); }
    catch (e) { setError(String(e)); }
  }

  useEffect(() => {
    refresh();
    request<Health & { status: string; ai_provider: string }>('/api/health')
      .then(setHealth).catch(e => setError(String(e)));
    refreshResearchHistory();
  }, []);

  useEffect(() => { activeRef.current = active; }, [active]);
  useEffect(() => { messagesRef.current = messages; }, [messages]);
  useEffect(() => {
    autoAnalyseRef.current = autoAnalyse;
    if (!autoAnalyse) {
      if (liveAnalysisTimerRef.current !== undefined) window.clearTimeout(liveAnalysisTimerRef.current);
      setLiveAnalysisStatus('Automatic analysis is off');
    } else {
      setLiveAnalysisStatus('Waiting for a pause in the conversation');
    }
  }, [autoAnalyse]);

  function scheduleLiveAnalysis(debate: Debate, transcript: Message[]) {
    if (!autoAnalyseRef.current || transcript.length === 0) return;
    if (liveAnalysisTimerRef.current !== undefined) window.clearTimeout(liveAnalysisTimerRef.current);
    setLiveAnalysisStatus('Waiting for a 10-second pause…');
    liveAnalysisTimerRef.current = window.setTimeout(() => {
      if (!autoAnalyseRef.current || activeRef.current?.id !== debate.id) return;
      const cooldown = 30000 - (Date.now() - lastLiveAnalysisAtRef.current);
      if (cooldown > 0) {
        setLiveAnalysisStatus(`Next automatic check in ${Math.ceil(cooldown / 1000)} seconds`);
        liveAnalysisTimerRef.current = window.setTimeout(() => {
          if (autoAnalyseRef.current && activeRef.current?.id === debate.id) void runAnalysis(debate, transcript, true);
        }, cooldown);
        return;
      }
      void runAnalysis(debate, transcript, true);
    }, 10000);
  }

  async function runAnalysis(debate: Debate, transcript: Message[], live: boolean) {
    if (transcript.length === 0) return;
    if (analysisBusyRef.current) {
      if (live) setLiveAnalysisStatus('An analysis is already running; the next pause will trigger another check');
      return;
    }
    analysisBusyRef.current = true;
    lastLiveAnalysisAtRef.current = Date.now();
    setBusy(true);
    if (live) setLiveAnalysisStatus('Checking the latest arguments and evidence…');
    setError('');
    try {
      const analysisMessages = transcript.slice(-(live ? 40 : 60));
      const latest = analysisMessages[analysisMessages.length - 1];
      const data = await request<Result>('/api/analyse', {
        method: 'POST',
        body: JSON.stringify({ topic: debate.topic, messages: analysisMessages, latest_speaker: latest.speaker, latest_statement: latest.content }),
      });
      const currentTranscript = messagesRef.current;
      const transcriptIsCurrent = currentTranscript.length === transcript.length &&
        currentTranscript.every((message, index) => message.id === transcript[index]?.id &&
          message.speaker === transcript[index]?.speaker && message.content === transcript[index]?.content);
      if (activeRef.current?.id === debate.id && transcriptIsCurrent) {
        setResult(data);
        setLastAnalysisAt(new Date().toLocaleTimeString());
      }
      if (live) setLiveAnalysisStatus(transcriptIsCurrent
        ? `Updated ${new Date().toLocaleTimeString()}`
        : 'New speech arrived during analysis; another check will follow after a pause');
    } catch (e) {
      if (live) setLiveAnalysisStatus(`Live analysis failed: ${String(e).replace(/^Error: /, '')}`);
      else setError(String(e));
    } finally {
      analysisBusyRef.current = false;
      setBusy(false);
      if (autoAnalyseRef.current && activeRef.current?.id === debate.id && messagesRef.current.length > transcript.length) {
        scheduleLiveAnalysis(debate, messagesRef.current);
      }
    }
  }

  function enqueueMessage(debateID: string, messageSpeaker: 'you' | 'opponent', content: string): Promise<Message> {
    const task = messageQueueRef.current.then(async () => {
      const message = await request<Message>(`/api/debates/${debateID}/messages`, {
        method: 'POST', body: JSON.stringify({ speaker: messageSpeaker, content }),
      });
      if (activeRef.current?.id === debateID) {
        const updated = [...messagesRef.current, message];
        messagesRef.current = updated;
        setMessages(updated);
        setResult(null);
        const currentDebate = activeRef.current;
        if (currentDebate) scheduleLiveAnalysis(currentDebate, updated);
      }
      return message;
    });
    messageQueueRef.current = task.then(() => undefined, error => { setError(String(error)); });
    return task;
  }

  async function flushSpeech() {
    if (speechFlushTimerRef.current !== undefined) window.clearTimeout(speechFlushTimerRef.current);
    speechFlushTimerRef.current = undefined;
    const pending = speechBufferRef.current;
    speechBufferRef.current = null;
    const debate = activeRef.current;
    if (!pending || !debate || !pending.content.trim()) return;
    await enqueueMessage(debate.id, pending.speaker, pending.content.trim());
  }

  function receiveSpeech(text: string, capturedSpeaker: 'you' | 'opponent') {
    const clean = text.trim();
    if (!clean || !activeRef.current) return;
    const pending = speechBufferRef.current;
    if (pending && pending.speaker !== capturedSpeaker) void flushSpeech();
    const current = speechBufferRef.current;
    speechBufferRef.current = current && current.speaker === capturedSpeaker
      ? { speaker: capturedSpeaker, content: `${current.content} ${clean}`.trim() }
      : { speaker: capturedSpeaker, content: clean };
    if (speechFlushTimerRef.current !== undefined) window.clearTimeout(speechFlushTimerRef.current);
    speechFlushTimerRef.current = window.setTimeout(() => { void flushSpeech().catch(e => setError(String(e))); }, 1400);
  }

  function changeSpeaker(next: 'you' | 'opponent') {
    if (speaker === next) return;
    void flushSpeech().catch(e => setError(String(e)));
    setSpeaker(next);
  }

  async function changeMessageSpeaker(message: Message, next: 'you' | 'opponent') {
    if (!active || !message.id || message.speaker === next) return;
    try {
      await request<{ id: string; speaker: string }>(`/api/debates/${active.id}/messages/${message.id}`, {
        method: 'PATCH', body: JSON.stringify({ speaker: next }),
      });
      const updated = messagesRef.current.map(item => item.id === message.id ? { ...item, speaker: next } : item);
      messagesRef.current = updated;
      setMessages(updated);
      setResult(null);
      setLedger(null);
      scheduleLiveAnalysis(active, updated);
    } catch (e) { setError(String(e)); }
  }

  async function create() {
    if (!title.trim() || !topic.trim()) return;
    try {
      await flushSpeech();
      const debate = await request<Debate>('/api/debates', {
        method: 'POST', body: JSON.stringify({ title, topic }),
      });
      activeRef.current = debate;
      messagesRef.current = [];
      setActive(debate); setMessages([]); setResult(null); setLedger(null); setTitle(''); setTopic(''); setTab('live');
      refresh();
    } catch (e) { setError(String(e)); }
  }

  async function select(debate: Debate) {
    try {
      await flushSpeech();
      if (liveAnalysisTimerRef.current !== undefined) window.clearTimeout(liveAnalysisTimerRef.current);
      activeRef.current = null;
      messagesRef.current = [];
      setActive(null); setMessages([]);
      const [loadedMessages, loadedLedger] = await Promise.all([
        request<Message[]>(`/api/debates/${debate.id}/messages`),
        request<LedgerReport>(`/api/debates/${debate.id}/ledger`),
      ]);
      activeRef.current = debate;
      messagesRef.current = loadedMessages;
      setActive(debate); setMessages(loadedMessages); setLedger(loadedLedger); setResult(null); setTab('live');
    } catch (e) { setError(String(e)); }
  }

  async function add() {
    if (!active || !draft.trim()) return;
    const content = draft.trim();
    setDraft('');
    try {
      await flushSpeech();
      await enqueueMessage(active.id, speaker, content);
    } catch (e) { setError(String(e)); }
  }

  async function analyse() {
    const debate = activeRef.current;
    if (!debate) return;
    try {
      await flushSpeech();
      await runAnalysis(debate, messagesRef.current, false);
    } catch (e) { setError(String(e)); }
  }

  async function updateLedger() {
    if (!active) return;
    try { await flushSpeech(); }
    catch (e) { setError(String(e)); return; }
    if (messagesRef.current.length === 0) return;
    setLedgerBusy(true); setError('');
    try {
      const report = await request<LedgerReport>(`/api/debates/${active.id}/ledger/analyse`, { method: 'POST' });
      setLedger(report);
    } catch (e) { setError(String(e)); }
    finally { setLedgerBusy(false); }
  }

  async function research() {
    if (!researchTopic.trim()) return;
    setResearchBusy(true); setError(''); setResearchReport(null);
    try {
      const report = await request<ResearchReport>('/api/research', {
        method: 'POST', body: JSON.stringify({ topic: researchTopic, position: researchPosition }),
      });
      setResearchReport(report); refreshResearchHistory();
    } catch (e) { setError(String(e)); }
    finally { setResearchBusy(false); }
  }

  async function openResearch(item: ResearchSummary) {
    try {
      const report = await request<ResearchReport>(`/api/research/${item.id}`);
      setResearchReport(report); setResearchTopic(report.topic); setResearchPosition(report.position || ''); setTab('research');
    } catch (e) { setError(String(e)); }
  }

  async function startDebateFromResearch() {
    if (!researchReport) return;
    try {
      await flushSpeech();
      const debate = await request<Debate>('/api/debates', {
        method: 'POST',
        body: JSON.stringify({ title: `Preparation: ${researchReport.topic}`.slice(0, 150), topic: researchReport.topic }),
      });
      activeRef.current = debate;
      messagesRef.current = [];
      setActive(debate); setMessages([]); setResult(null); setLedger(null); setTab('live'); refresh();
    } catch (e) { setError(String(e)); }
  }

  const opponentCount = useMemo(() => result?.opponent_findings?.length || 0, [result]);

  return <div className="shell">
    <aside className="sidebar">
      <div className="brand"><div className="brand-icon"><Gavel size={19} /></div><div><b>DEBATE</b><small>INTELLIGENCE</small></div></div>
      <div className="nav-label">WORKSPACE</div>
      <button className={tab === 'live' ? 'nav active' : 'nav'} onClick={() => setTab('live')}><Radio size={17} /> Live debate</button>
      <button className={tab === 'research' ? 'nav active' : 'nav'} onClick={() => setTab('research')}><BookOpen size={17} /> Preparation</button>
      <button className={tab === 'ledger' ? 'nav active' : 'nav'} onClick={() => setTab('ledger')}><ListChecks size={17} /> Argument ledger</button>
      <button className={tab === 'history' ? 'nav active' : 'nav'} onClick={() => setTab('history')}><History size={17} /> Debate history</button>
      <div className="nav-label recent">RECENT DEBATES</div>
      {debates.slice(0, 8).map(debate => <button key={debate.id} className="debate-link" onClick={() => select(debate)}>{debate.title}</button>)}
      <div className="sidebar-bottom"><span className="dot" /> API workspace <small>Stage 5 · Live intelligence</small></div>
    </aside>

    <main className="main">
      <header>
        <div>
          <div className="eyebrow">BRITISH POLITICS · DEBATE WORKSPACE</div>
          <h1>{tab === 'history' ? 'Debate history' : tab === 'research' ? 'Preparation workspace' : tab === 'ledger' ? 'Argument ledger' : active?.title || 'Live debate'}</h1>
          <p>{active ? active.topic : 'Prepare, research and analyse political arguments.'}</p>
        </div>
        <div className="header-status"><span className="dot" /> {health ? (health.gemini_configured ? 'GEMINI CONNECTED' : 'GEMINI KEY MISSING') : 'CHECKING API'}</div>
      </header>

      {error && <div className="error"><CircleAlert size={16} />{error}<button onClick={() => setError('')}>Dismiss</button></div>}

      {tab === 'history' ? <section className="panel history">
        <h2><History size={18} /> Previous debates</h2>
        {debates.length === 0 ? <p className="muted">No saved debates yet.</p> : debates.map(debate =>
          <button key={debate.id} className="history-row" onClick={() => select(debate)}>
            <div><b>{debate.title}</b><small>{debate.topic}</small></div><span>Open →</span>
          </button>)}
        <h2 className="history-subhead"><BookOpen size={18} /> Saved research briefs</h2>
        {researchHistory.length === 0 ? <p className="muted">No research briefs saved yet.</p> : researchHistory.map(item =>
          <button key={item.id} className="history-row" onClick={() => openResearch(item)}>
            <div><b>{item.topic}</b><small>{item.position ? `Position: ${item.position}` : 'Balanced research brief'}</small></div><span>Open →</span>
          </button>)}
      </section> : tab === 'research' ? <section className="research-layout">
        <div className="panel research-form">
          <div className="panel-heading"><BookOpen /><h2>Build a debate research brief</h2></div>
          <p className="muted">Search current sources, then ask Gemini to organise the evidence into arguments, counterarguments and questions.</p>
          <label>Topic or proposition</label>
          <input value={researchTopic} onChange={e => setResearchTopic(e.target.value)} placeholder="e.g. Should the UK abolish tuition fees?" />
          <label>Your position <span className="optional">(optional)</span></label>
          <textarea className="position-input" value={researchPosition} onChange={e => setResearchPosition(e.target.value)} placeholder="e.g. I support abolishing tuition fees because..." />
          <button className="primary research-button" onClick={research} disabled={researchBusy || !researchTopic.trim()}>
            {researchBusy ? <span className="spinner" /> : <Search size={16} />}{researchBusy ? 'Researching sources…' : 'Build research brief'}
          </button>
          {!health?.web_research_configured && <div className="callout warning"><CircleAlert size={17} /> Add a <code>TAVILY_API_KEY</code> to the backend environment to enable web research. Gemini analysis can still run without it.</div>}
          <div className="research-notes">
            <b>What this does</b>
            <ul>
              <li>Searches for official and independent UK evidence.</li>
              <li>Separates established facts from contested or incomplete claims.</li>
              <li>Builds the strongest arguments on both sides.</li>
              <li>Flags devolution, definitions and missing context where relevant.</li>
            </ul>
          </div>
        </div>

        {researchReport ? <ResearchBrief report={researchReport} onStartDebate={startDebateFromResearch} /> :
          <div className="panel research-placeholder"><div className="empty-icon"><FileSearch /></div><h3>Your research brief will appear here</h3><p>Enter a topic and generate a source-backed preparation pack. Previous reports are available in Debate history.</p></div>}
      </section> : tab === 'ledger' ? <LedgerWorkspace
        debate={active} report={ledger} messages={messages} busy={ledgerBusy} onUpdate={updateLedger}
      /> : <div className="workspace">
        <section className="panel transcript">
          <div className="panel-heading"><MessageSquareText /><h2>Conversation</h2><span className="count">{messages.length} entries</span></div>
          {!active ? <div className="empty">
            <div className="empty-icon"><Gavel /></div><h3>Start a debate session</h3><p>Create a topic and begin entering statements from either participant.</p>
            <div className="create-form">
              <input value={title} onChange={e => setTitle(e.target.value)} placeholder="Debate title" />
              <input value={topic} onChange={e => setTopic(e.target.value)} placeholder="Debate topic or proposition" />
              <button className="primary" onClick={create}><Plus size={16} /> Create debate</button>
            </div>
          </div> : <>
            <SpeechCapture
              sessionId={active.id}
              speaker={speaker}
              onSpeakerChange={changeSpeaker}
              onTranscript={receiveSpeech}
            />
            <div className="messages">
              {messages.map((message, index) => <div key={message.id || index} className={`message ${message.speaker}`}>
                <div className="message-meta">
                  <span>{message.speaker === 'you' ? 'YOU' : 'OPPONENT'}</span>
                  <small>Statement {index + 1}</small>
                  {message.id && <select className="speaker-correction" value={message.speaker} onChange={e => void changeMessageSpeaker(message, e.target.value as 'you' | 'opponent')} aria-label={`Correct speaker for statement ${index + 1}`}>
                    <option value="you">You</option><option value="opponent">Opponent</option>
                  </select>}
                </div>
                <p>{message.content}</p>
              </div>)}
              {messages.length === 0 && <div className="empty compact"><p>No statements yet. Add the first contribution below.</p></div>}
            </div>
            <div className="composer">
              <div className="manual-speaker-label">SPEAKER FOR MANUAL ENTRY</div>
              <div className="speaker-switch">
                <button className={speaker === 'opponent' ? 'selected opponent' : ''} onClick={() => changeSpeaker('opponent')}>Opponent</button>
                <button className={speaker === 'you' ? 'selected you' : ''} onClick={() => changeSpeaker('you')}>You</button>
              </div>
              <textarea value={draft} onChange={e => setDraft(e.target.value)} placeholder="Enter the statement or argument..." onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); add(); } }} />
              <button className="primary send" onClick={add} disabled={!draft.trim()}><Send size={16} /> Add statement</button>
            </div>
            <div className="transcript-footer stage5-footer">
              <div className="analysis-actions">
                <button className="analyse" onClick={analyse} disabled={busy || messages.length === 0}>{busy ? <span className="spinner" /> : <BrainCircuit size={17} />}{busy ? 'Analysing…' : 'Analyse conversation now'}</button>
                <label className="auto-analysis-toggle">
                  <input type="checkbox" checked={autoAnalyse} onChange={e => setAutoAnalyse(e.target.checked)} />
                  <span><b>Automatic live analysis</b><small>{liveAnalysisStatus}</small></span>
                </label>
              </div>
              <span className="analysis-usage-note">{lastAnalysisAt ? `Last analysis: ${lastAnalysisAt}. ` : ''}{health?.web_research_configured ? 'Automatic checks wait for 10 seconds of silence and run at most once every 30 seconds; each check may use Gemini and Tavily.' : 'Add Tavily for source retrieval. Automatic checks wait for 10 seconds of silence and run at most once every 30 seconds.'}</span>
            </div>
          </>}
        </section>

        <section className="insights">
          <div className="panel insight-panel">
            <div className="panel-heading"><ShieldAlert /><h2>Opponent analysis</h2><span className="count">{opponentCount}</span></div>
            {result ? result.opponent_findings?.length ? result.opponent_findings.map((finding, i) => <FindingCard key={i} finding={finding} sources={result.sources || []} />) : <p className="muted">No clear issues detected in the supplied statements.</p> : <Placeholder text="Potential factual errors, weak reasoning and contradictions will appear here." />}
          </div>
          <div className="panel insight-panel">
            <div className="panel-heading"><Activity /><h2>Your analysis</h2><span className="count">{result?.your_findings?.length || 0}</span></div>
            {result ? result.your_findings?.length ? result.your_findings.map((finding, i) => <FindingCard key={i} finding={finding} sources={result.sources || []} />) : <p className="muted">No clear issues detected in your supplied statements.</p> : <Placeholder text="Your unsupported claims, inaccuracies and possible improvements will appear here too." />}
          </div>
          <div className="panel insight-panel">
            <div className="panel-heading"><Sparkles /><h2>Suggested responses</h2></div>
            {result ? <>
              <p className="analysis-summary">{result.summary}</p>
              <ul className="suggestions">{(result.suggestions || []).map((item, i) => <li key={i}>{item}</li>)}</ul>
              {(result.evidence_needed || []).length > 0 && <><h3 className="mini-heading">Evidence still needed</h3><ul className="suggestions">{result.evidence_needed.map((item, i) => <li key={i}>{item}</li>)}</ul></>}
              {result.research_warning && <p className="research-warning">{result.research_warning}</p>}
              <p className="disclaimer">{result.disclaimer}</p>
            </> : <Placeholder text="Concise rebuttals, alternative approaches and probing questions will appear here." />}
          </div>
          {result && <div className="panel insight-panel source-panel">
            <div className="panel-heading"><FileSearch /><h2>Evidence sources</h2><span className="count">{result.sources?.length || 0}</span></div>
            {result.sources?.length ? <SourceList sources={result.sources} /> : <p className="muted">No live sources were retrieved for this analysis. Treat factual assessments as unverified.</p>}
          </div>}
        </section>
      </div>}
    </main>
  </div>;
}

function LedgerWorkspace({
  debate, report, messages, busy, onUpdate,
}: {
  debate: Debate | null; report: LedgerReport | null; messages: Message[];
  busy: boolean; onUpdate: () => void;
}) {
  if (!debate) return <section className="panel ledger-empty">
    <div className="empty-icon"><ListChecks /></div>
    <h2>Select a debate first</h2>
    <p>Open a saved debate from the sidebar or create one in the Live debate workspace. The ledger follows that debate's transcript.</p>
  </section>;

  const claims = report?.claims || [];
  const youClaims = claims.filter(claim => claim.speaker === 'you');
  const opponentClaims = claims.filter(claim => claim.speaker === 'opponent');
  const claimByID = new Map<string, LedgerClaim>(claims.map(claim => [claim.id, claim] as const));
  const stale = Boolean(report?.updated_at && report.message_count !== messages.length);
  const supportedCount = claims.filter(claim => claim.assessment === 'supported').length;
  const contestedCount = claims.filter(claim => ['contested', 'contradicted'].includes(claim.assessment)).length;
  const unverifiedCount = claims.filter(claim => claim.assessment === 'unverified' || claim.assessment === 'context_needed').length;

  return <div className="ledger-layout">
    <section className="panel ledger-intro">
      <div className="ledger-intro-copy">
        <div className="panel-heading"><ListChecks /><h2>{debate.title}</h2></div>
        <p className="muted">A running record of important claims, their evidence and possible contradictions across this transcript.</p>
        <small className="ledger-topic">{debate.topic}</small>
      </div>
      <button className="primary ledger-update" onClick={onUpdate} disabled={busy || messages.length === 0}>
        {busy ? <span className="spinner" /> : <BrainCircuit size={16} />}
        {busy ? 'Updating ledger…' : report?.updated_at ? 'Refresh argument ledger' : 'Build argument ledger'}
      </button>
      {messages.length === 0 && <p className="muted">Add transcript statements before building the ledger.</p>}
      {stale && <div className="ledger-stale"><CircleAlert size={15} /> The transcript has changed since this ledger was last updated. Refresh it to include the latest statements.</div>}
      {report?.updated_at && <small className="ledger-updated">Last updated {new Date(report.updated_at).toLocaleString()} · {report.message_count} of {messages.length} current statements analysed</small>}
    </section>

    {report?.warnings?.map((warning, index) => <div className="ledger-warning" key={index}><CircleAlert size={15} />{warning}</div>)}

    <section className="ledger-stats">
      <div className="panel ledger-stat"><b>{claims.length}</b><span>Tracked claims</span></div>
      <div className="panel ledger-stat"><b>{report?.contradictions?.length || 0}</b><span>Potential contradictions</span></div>
      <div className="panel ledger-stat"><b>{supportedCount}</b><span>Supported by retrieved sources</span></div>
      <div className="panel ledger-stat"><b>{contestedCount + unverifiedCount}</b><span>Need scrutiny or context</span></div>
    </section>

    <section className="panel ledger-claims-panel">
      <div className="panel-heading"><Scale /><h2>Claim register</h2><span className="count">{claims.length}</span></div>
      {!report?.updated_at ? <div className="placeholder"><FileSearch size={22} /><p>Build the ledger to extract important claims from the full transcript and assess them against retrieved sources.</p></div> : claims.length === 0 ? <p className="muted">No substantive claims were identified in this transcript.</p> : <div className="ledger-columns">
        <div className="ledger-column"><h3 className="ledger-speaker you">YOUR CLAIMS <span>{youClaims.length}</span></h3>
          {youClaims.length ? youClaims.map(claim => <LedgerClaimCard key={claim.id} claim={claim} messages={messages} />) : <p className="muted">No claims attributed to you.</p>}
        </div>
        <div className="ledger-column"><h3 className="ledger-speaker opponent">OPPONENT CLAIMS <span>{opponentClaims.length}</span></h3>
          {opponentClaims.length ? opponentClaims.map(claim => <LedgerClaimCard key={claim.id} claim={claim} messages={messages} />) : <p className="muted">No claims attributed to your opponent.</p>}
        </div>
      </div>}
      <p className="disclaimer">Evidence assessments are provisional and use search-result snippets, not complete source verification. A missing source does not prove a claim false. Review the linked material and original transcript before relying on a status or contradiction.</p>
    </section>

    <section className="panel ledger-contradictions-panel">
      <div className="panel-heading"><Scale /><h2>Potential contradictions</h2><span className="count">{report?.contradictions?.length || 0}</span></div>
      {report?.contradictions?.length ? report.contradictions.map(item => {
        const a = claimByID.get(item.claim_a_id);
        const b = claimByID.get(item.claim_b_id);
        if (!a || !b) return null;
        return <article className="contradiction" key={item.id}>
          <div className="contradiction-pair">
            <div className="contradiction-claim"><small>{a.speaker === 'you' ? 'YOU' : 'OPPONENT'}</small><p>{a.text}</p></div>
            <div className="contradiction-vs">CONFLICTS WITH</div>
            <div className="contradiction-claim"><small>{b.speaker === 'you' ? 'YOU' : 'OPPONENT'}</small><p>{b.text}</p></div>
          </div>
          <p className="contradiction-explanation">{item.explanation}</p>
          <small className="confidence">{item.confidence} confidence · Review the original statements and context before treating this as a contradiction.</small>
        </article>;
      }) : <p className="muted">No clear contradictions have been identified. This does not mean every claim is consistent or correct; it means the model did not find a sufficiently clear conflict.</p>}
    </section>
  </div>;
}

function LedgerClaimCard({ claim, messages }: { claim: LedgerClaim; messages: Message[] }) {
  const statementNumbers = claim.message_ids
    .map(id => messages.findIndex(message => message.id === id) + 1)
    .filter(number => number > 0);
  return <article className="ledger-claim">
    <div className="ledger-claim-top"><span className={`assessment ${claim.assessment}`}>{claim.assessment.replace(/_/g, ' ')}</span><small>{claim.confidence} confidence</small></div>
    <b>{claim.text}</b>
    <div className="ledger-claim-meta"><span>{claim.claim_type.replace(/_/g, ' ')}</span>{statementNumbers.length > 0 && <span>Statement{statementNumbers.length > 1 ? 's' : ''} {statementNumbers.join(', ')}</span>}</div>
    <p>{claim.explanation}</p>
    {Boolean(claim.evidence?.length) && <div className="claim-evidence-list">
      {claim.evidence.map(item => <div className="claim-evidence" key={item.id}>
        <div className="claim-evidence-meta"><span className={`evidence-relation ${item.relationship}`}>{item.relationship}</span><small>{item.domain || item.url}{item.published_at ? ` · ${item.published_at}` : ''}</small></div>
        <a href={item.url} target="_blank" rel="noreferrer">{item.title} <ExternalLink size={11} /></a>
        <p>{item.explanation}</p>
        <small className="evidence-snippet">{item.snippet}</small>
      </div>)}
    </div>}
  </article>;
}

function Placeholder({ text }: { text: string }) {
  return <div className="placeholder"><FileSearch size={22} /><p>{text}</p></div>;
}

function SourceLinks({ ids, sources }: { ids?: string[]; sources: Source[] }) {
  if (!ids?.length) return null;
  const matched = ids.map(id => sources.find(source => source.id === id)).filter((source): source is Source => Boolean(source));
  if (!matched.length) return null;
  return <div className="inline-sources"><small>REFERENCES</small>{matched.map(source => <a key={source.id} href={source.url} target="_blank" rel="noreferrer">{source.id} <ExternalLink size={10} /></a>)}</div>;
}

function FindingCard({ finding, sources }: { finding: Finding; sources: Source[] }) {
  return <article className="finding">
    <div className="finding-top"><span className={`severity ${finding.severity}`}>{finding.severity}</span><span className="confidence">{finding.confidence} confidence</span></div>
    <b>{finding.title}</b><p>{finding.explanation}</p>
    {finding.suggested_response && <div className="response"><small>SUGGESTED RESPONSE</small>{finding.suggested_response}</div>}
    {finding.question && <div className="question"><small>FOLLOW-UP QUESTION</small>{finding.question}</div>}
    <SourceLinks ids={finding.source_ids} sources={sources} />
  </article>;
}

function SourceList({ sources }: { sources: Source[] }) {
  return <div className="source-list">{sources.map(source => <article className="source-item" key={source.id}>
    <div className="source-meta"><span>{source.id}</span><small>{source.domain || source.url}{source.published_at ? ` · ${source.published_at}` : ''}</small></div>
    <a href={source.url} target="_blank" rel="noreferrer">{source.title} <ExternalLink size={12} /></a>
    <p>{source.snippet}</p>
  </article>)}</div>;
}

function ResearchBrief({ report, onStartDebate }: { report: ResearchReport; onStartDebate: () => void }) {
  const sources = report.sources || [];
  return <div className="research-results">
    <section className="panel brief-overview">
      <div className="brief-topline"><span className="brief-tag"><CheckCircle2 size={13} /> RESEARCH BRIEF</span><small>{report.generated_at ? new Date(report.generated_at).toLocaleString() : ''}</small></div>
      <h2>{report.topic}</h2>
      {report.position && <p className="position-label">Your stated position: {report.position}</p>}
      <p className="overview-text">{report.overview}</p>
      <button className="primary" onClick={onStartDebate}><Plus size={15} /> Start a debate with this topic</button>
    </section>

    <div className="argument-grid">
      <section className="panel argument-panel"><div className="panel-heading"><Scale /><h2>Arguments in favour</h2></div>
        {(report.arguments_for || []).map((argument, index) => <article className="argument" key={index}><b>{argument.title}</b><p>{argument.reasoning}</p>{argument.evidence && <p className="argument-evidence">{argument.evidence}</p>}<SourceLinks ids={argument.source_ids} sources={sources} /></article>)}
      </section>
      <section className="panel argument-panel"><div className="panel-heading"><Scale /><h2>Arguments against</h2></div>
        {(report.arguments_against || []).map((argument, index) => <article className="argument" key={index}><b>{argument.title}</b><p>{argument.reasoning}</p>{argument.evidence && <p className="argument-evidence">{argument.evidence}</p>}<SourceLinks ids={argument.source_ids} sources={sources} /></article>)}
      </section>
    </div>

    <section className="panel facts-panel"><div className="panel-heading"><CheckCircle2 /><h2>Key claims and evidence</h2></div>
      {(report.key_facts || []).map((fact, index) => <article className="fact" key={index}>
        <div className="fact-heading"><b>{fact.claim}</b><span className={`assessment ${fact.assessment}`}>{fact.assessment.replace(/_/g, ' ')}</span></div>
        <p>{fact.context}</p><div className="fact-foot"><small>{fact.confidence} confidence</small><SourceLinks ids={fact.source_ids} sources={sources} /></div>
      </article>)}
    </section>

    <div className="prep-grid">
      <section className="panel"><div className="panel-heading"><BrainCircuit /><h2>Devil's advocate</h2></div><p>{report.devils_advocate || 'No devil’s-advocate assessment was generated.'}</p></section>
      <section className="panel"><div className="panel-heading"><Sparkles /><h2>Debate strategy</h2></div><BulletList items={report.debate_strategy} /></section>
      <section className="panel"><div className="panel-heading"><MessageSquareText /><h2>Opening statement</h2></div><p>{report.opening_statement || 'No opening statement generated.'}</p></section>
      <section className="panel"><div className="panel-heading"><Gavel /><h2>Closing statement</h2></div><p>{report.closing_statement || 'No closing statement generated.'}</p></section>
      <section className="panel"><div className="panel-heading"><Lightbulb /><h2>Counterarguments to expect</h2></div><BulletList items={report.counterarguments_to_expect} /></section>
      <section className="panel"><div className="panel-heading"><ShieldAlert /><h2>Risks to your position</h2></div><BulletList items={report.risks_to_your_position} /></section>
      <section className="panel"><div className="panel-heading"><MessageSquareText /><h2>Questions to ask</h2></div><BulletList items={report.questions_to_ask} /></section>
      <section className="panel"><div className="panel-heading"><CircleAlert /><h2>Caveats and gaps</h2></div><BulletList items={report.caveats} /></section>
      <section className="panel"><div className="panel-heading"><FileSearch /><h2>Evidence still needed</h2></div><BulletList items={report.evidence_gaps} /></section>
    </div>

    <section className="panel"><div className="panel-heading"><ShieldAlert /><h2>Rebuttal bank</h2></div>
      {(report.rebuttal_bank || []).length ? report.rebuttal_bank!.map((item, index) => <article className="argument" key={index}><b>{item.challenge}</b><p>{item.response}</p>{item.caveat && <p className="argument-evidence">Caveat: {item.caveat}</p>}<SourceLinks ids={item.source_ids} sources={sources} /></article>) : <p className="muted">No rebuttals generated.</p>}
    </section>

    <section className="panel"><div className="panel-heading"><FileSearch /><h2>Research sources</h2><span className="count">{sources.length}</span></div><SourceList sources={sources} /><p className="disclaimer">{report.disclaimer}</p></section>
  </div>;
}

function BulletList({ items }: { items?: string[] }) {
  if (!items?.length) return <p className="muted">No specific items identified.</p>;
  return <ul className="brief-bullets">{items.map((item, index) => <li key={index}>{item}</li>)}</ul>;
}
