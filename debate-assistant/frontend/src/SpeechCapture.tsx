import { useEffect, useRef, useState } from 'react';
import { Mic, MicOff, AlertTriangle, Radio } from 'lucide-react';

type Speaker = 'you' | 'opponent';
type SpeechAlternativeLike = { transcript: string };
type SpeechResultLike = { isFinal: boolean; 0: SpeechAlternativeLike };
type SpeechEventLike = { resultIndex: number; results: ArrayLike<SpeechResultLike> };
type SpeechErrorLike = { error: string };
type SpeechRecognitionLike = {
  continuous: boolean;
  interimResults: boolean;
  lang: string;
  onstart: (() => void) | null;
  onresult: ((event: SpeechEventLike) => void) | null;
  onerror: ((event: SpeechErrorLike) => void) | null;
  onend: (() => void) | null;
  start: () => void;
  stop: () => void;
};
type SpeechWindow = Window & {
  SpeechRecognition?: new () => SpeechRecognitionLike;
  webkitSpeechRecognition?: new () => SpeechRecognitionLike;
};

type Props = {
  sessionId: string;
  speaker: Speaker;
  disabled?: boolean;
  onSpeakerChange: (speaker: Speaker) => void;
  onTranscript: (text: string, speaker: Speaker) => void;
};

export default function SpeechCapture({ sessionId, speaker, disabled = false, onSpeakerChange, onTranscript }: Props) {
  const [supported, setSupported] = useState<boolean | null>(null);
  const [recording, setRecording] = useState(false);
  const [interim, setInterim] = useState('');
  const [captureError, setCaptureError] = useState('');
  const recognitionRef = useRef<SpeechRecognitionLike | null>(null);
  const shouldListenRef = useRef(false);
  const speakerRef = useRef(speaker);
  const transcriptCallbackRef = useRef(onTranscript);
  const restartTimerRef = useRef<number | undefined>(undefined);

  useEffect(() => { speakerRef.current = speaker; }, [speaker]);
  useEffect(() => { transcriptCallbackRef.current = onTranscript; }, [onTranscript]);

  useEffect(() => {
    const browser = window as SpeechWindow;
    setSupported(Boolean(browser.SpeechRecognition || browser.webkitSpeechRecognition));
  }, []);

  useEffect(() => {
    shouldListenRef.current = false;
    if (restartTimerRef.current !== undefined) window.clearTimeout(restartTimerRef.current);
    if (recognitionRef.current) {
      recognitionRef.current.onend = null;
      recognitionRef.current.onresult = null;
      recognitionRef.current.onerror = null;
      recognitionRef.current.stop();
    }
    recognitionRef.current = null;
    setRecording(false);
    setInterim('');
    return () => {
      shouldListenRef.current = false;
      if (restartTimerRef.current !== undefined) window.clearTimeout(restartTimerRef.current);
      if (recognitionRef.current) {
        recognitionRef.current.onend = null;
        recognitionRef.current.onresult = null;
        recognitionRef.current.stop();
      }
      recognitionRef.current = null;
    };
  }, [sessionId]);

  function stopCapture() {
    shouldListenRef.current = false;
    if (restartTimerRef.current !== undefined) window.clearTimeout(restartTimerRef.current);
    if (recognitionRef.current) {
      recognitionRef.current.onend = null;
      recognitionRef.current.onresult = null;
      recognitionRef.current.onerror = null;
      recognitionRef.current.stop();
    }
    recognitionRef.current = null;
    setRecording(false);
    setInterim('');
  }

  function startCapture() {
    if (disabled) return;
    const browser = window as SpeechWindow;
    const Recognition = browser.SpeechRecognition || browser.webkitSpeechRecognition;
    if (!Recognition) {
      setSupported(false);
      setCaptureError('Speech recognition is not available in this browser. Use Chrome or Edge, or enter statements manually.');
      return;
    }

    setCaptureError('');
    if (recognitionRef.current) {
      recognitionRef.current.onend = null;
      recognitionRef.current.onresult = null;
      recognitionRef.current.onerror = null;
      recognitionRef.current.stop();
    }
    const recognition = new Recognition();
    recognition.lang = 'en-GB';
    recognition.continuous = true;
    recognition.interimResults = true;
    recognitionRef.current = recognition;
    shouldListenRef.current = true;

    recognition.onstart = () => setRecording(true);
    recognition.onresult = event => {
      let finalText = '';
      let interimText = '';
      for (let index = event.resultIndex; index < event.results.length; index += 1) {
        const result = event.results[index];
        const text = result[0]?.transcript || '';
        if (result.isFinal) finalText += `${text} `;
        else interimText += `${text} `;
      }
      setInterim(interimText.trim());
      if (finalText.trim()) {
        setCaptureError('');
        transcriptCallbackRef.current(finalText.trim(), speakerRef.current);
      }
    };
    recognition.onerror = event => {
      const fatal = ['not-allowed', 'service-not-allowed', 'audio-capture', 'network'].includes(event.error);
      const messages: Record<string, string> = {
        'not-allowed': 'Microphone access was denied. Allow microphone access in your browser settings.',
        'service-not-allowed': 'The browser speech service is unavailable or blocked.',
        'audio-capture': 'No microphone could be found. Check your microphone connection.',
        network: 'The browser speech service encountered a network error. Check your connection and try again.',
        'no-speech': 'No speech was detected. Keep speaking or check your microphone level.',
      };
      setCaptureError(messages[event.error] || `Speech recognition error: ${event.error}`);
      if (fatal) {
        shouldListenRef.current = false;
        setRecording(false);
      }
    };
    recognition.onend = () => {
      if (!shouldListenRef.current) {
        setRecording(false);
        return;
      }
      restartTimerRef.current = window.setTimeout(() => {
        if (!shouldListenRef.current) return;
        try { recognition.start(); }
        catch { setCaptureError('Speech recognition stopped unexpectedly. Press Start microphone to try again.'); shouldListenRef.current = false; setRecording(false); }
      }, 350);
    };

    try { recognition.start(); }
    catch {
      shouldListenRef.current = false;
      setRecording(false);
      setCaptureError('The microphone could not be started. Check browser permissions and try again.');
    }
  }

  return <section className="capture-panel">
    <div className="capture-topline">
      <div className="capture-title"><Radio size={15} /><b>LIVE TRANSCRIPTION</b><span className={recording ? 'capture-status recording' : 'capture-status'}>{recording ? 'LISTENING' : 'READY'}</span></div>
      <button className={recording ? 'capture-button stop' : 'capture-button'} onClick={recording ? stopCapture : startCapture} disabled={disabled || supported === false}>
        {recording ? <><MicOff size={15} /> Stop microphone</> : <><Mic size={15} /> Start microphone</>}
      </button>
    </div>
    <div className="capture-controls">
      <div>
        <small>WHO IS SPEAKING?</small>
        <div className="speaker-switch capture-speaker-switch">
          <button className={speaker === 'opponent' ? 'selected opponent' : ''} onClick={() => onSpeakerChange('opponent')} disabled={disabled}>Opponent</button>
          <button className={speaker === 'you' ? 'selected you' : ''} onClick={() => onSpeakerChange('you')} disabled={disabled}>You</button>
        </div>
      </div>
      <div className="capture-hint">Change the speaker selection when the other person takes over. You can correct labels in the transcript afterwards.</div>
    </div>
    {interim && <div className="capture-interim"><span>HEARING</span> {interim}</div>}
    {captureError && <div className="capture-error"><AlertTriangle size={14} />{captureError}</div>}
    {supported === false && <div className="capture-error"><AlertTriangle size={14} />This browser does not support the Web Speech API. Chrome or Edge is recommended; manual transcript entry remains available.</div>}
    <p className="capture-privacy">Microphone audio is handled by your browser's speech-recognition service. This app's backend receives and stores the recognised text, not the raw audio.</p>
  </section>;
}
