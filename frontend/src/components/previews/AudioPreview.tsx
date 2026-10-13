import React, { useState, useEffect, useRef } from 'react';
import { Play, Pause, Volume2, VolumeX, Music, ExternalLink } from 'lucide-react';
import { FileItem, formatFileSize } from '../../types';
import { getScopedStreamURL, warmupFile, getWarmupStatus, saveFileProgress } from '../../api/client';
import { formatTime, getBufferedPercent } from './utils';

export interface AudioPreviewProps {
  file: FileItem;
  onDockToBottomPlayer?: (file: FileItem) => void;
  onClose: () => void;
}

export const AudioPreview: React.FC<AudioPreviewProps> = ({
  file,
  onDockToBottomPlayer,
  onClose,
}) => {
  const audioRef = useRef<HTMLAudioElement>(null);

  const [isPlayingAudio, setIsPlayingAudio] = useState(false);
  const [isAudioBuffering, setIsAudioBuffering] = useState(false);
  const [audioProgress, setAudioProgress] = useState(0);
  const [audioDuration, setAudioDuration] = useState(0);
  const [audioVolume, setAudioVolume] = useState(1);
  const [isAudioMuted, setIsAudioMuted] = useState(false);
  const lastSaveTimeRef = useRef<number>(0);
  const [audioBuffered, setAudioBuffered] = useState(0);
  const [audioSrc, setAudioSrc] = useState<string>('');

  useEffect(() => {
    let cancelled = false;
    getScopedStreamURL(file.id)
      .then((url) => {
        if (!cancelled) setAudioSrc(url);
      })
      .catch((err) => console.warn('Failed to load scoped audio stream URL:', err));
    return () => {
      cancelled = true;
    };
  }, [file.id]);

  // Warmup detection for multi-chunk audio files
  const effectiveChunkSize = file.chunk_size || 16 * 1024 * 1024;
  const isMultiChunk = file.size > effectiveChunkSize;
  const [isWarming, setIsWarming] = useState(false);
  const [warmupReady, setWarmupReady] = useState(!isMultiChunk);
  const [warmupProgress, setWarmupProgress] = useState('');
  const warmupPollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    if (!isMultiChunk) {
      setWarmupReady(true);
      return;
    }

    let cancelled = false;
    setIsWarming(true);
    const targetChunks = Math.min(2, Math.ceil(file.size / effectiveChunkSize));
    setWarmupProgress(`Buffering initial chunks (0/${targetChunks})...`);

    const startWarmup = async () => {
      try {
        const status = await warmupFile(file.id);
        if (cancelled) return;

        const target = Math.min(2, status.total_chunks);
        const currentCached = Math.min(status.cached_count, target);

        if (status.ready) {
          setWarmupReady(true);
          setIsWarming(false);
          setWarmupProgress('');
          return;
        }

        setWarmupProgress(`Buffering initial chunks (${currentCached}/${target})...`);

        warmupPollRef.current = setInterval(async () => {
          if (cancelled) return;
          try {
            const st = await getWarmupStatus(file.id);
            if (cancelled) return;
            const tgt = Math.min(2, st.total_chunks);
            const cnt = Math.min(st.cached_count, tgt);
            setWarmupProgress(`Buffering initial chunks (${cnt}/${tgt})...`);

            if (st.ready) {
              setWarmupReady(true);
              setIsWarming(false);
              setWarmupProgress('');
              if (warmupPollRef.current) clearInterval(warmupPollRef.current);
            }
          } catch {
            // Ignore poll errors
          }
        }, 1000);
      } catch (err) {
        if (cancelled) return;
        console.warn('Audio warmup failed, loading directly:', err);
        setWarmupReady(true);
        setIsWarming(false);
      }
    };

    startWarmup();

    return () => {
      cancelled = true;
      if (warmupPollRef.current) clearInterval(warmupPollRef.current);
    };
  }, [file.id, file.size, isMultiChunk]);

  const togglePlayAudio = () => {
    if (!audioRef.current) return;
    if (isPlayingAudio) {
      audioRef.current.pause();
      setIsPlayingAudio(false);
    } else {
      audioRef.current.play().catch(() => {});
      setIsPlayingAudio(true);
    }
  };

  const handleAudioSeek = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = parseFloat(e.target.value);
    setAudioProgress(val);
    if (audioRef.current) {
      setIsAudioBuffering(true);
      audioRef.current.currentTime = val;
    }
  };

  if (isWarming && !warmupReady) {
    return (
      <div className="preview-audio-viewport">
        <div className="video-warmup-overlay">
          <div className="warmup-spinner" />
          <p className="warmup-text">{warmupProgress || 'Preparing audio stream...'}</p>
          <p className="warmup-subtext">Decrypting chunks from Telegram CDN</p>
        </div>
      </div>
    );
  }

  return (
    <div className="preview-audio-viewport">
      <audio
        ref={audioRef}
        src={audioSrc || undefined}
        onError={(e) => {
          console.error('Audio stream playback error:', e);
          setIsAudioBuffering(false);
        }}
        onPlay={() => {
          setIsPlayingAudio(true);
          setIsAudioBuffering(false);
        }}
        onPause={() => {
          setIsPlayingAudio(false);
          const a = audioRef.current;
          if (a && a.duration > 0) {
            lastSaveTimeRef.current = Date.now();
            saveFileProgress(file.id, a.currentTime / a.duration).catch(console.error);
          }
        }}
        onSeeking={() => setIsAudioBuffering(true)}
        onSeeked={() => {
          const a = audioRef.current;
          if (a && a.readyState >= 3) {
            setIsAudioBuffering(false);
          }
        }}
        onWaiting={() => setIsAudioBuffering(true)}
        onStalled={() => {
          const a = audioRef.current;
          if (a && !a.paused) setIsAudioBuffering(true);
        }}
        onPlaying={() => {
          setIsPlayingAudio(true);
          setIsAudioBuffering(false);
        }}
        onCanPlay={() => setIsAudioBuffering(false)}
        onCanPlayThrough={() => setIsAudioBuffering(false)}
        onTimeUpdate={() => {
          const a = audioRef.current;
          if (a) {
            setAudioProgress(a.currentTime);
            setAudioDuration(a.duration || 0);
            setAudioBuffered(getBufferedPercent(a));
            if (isAudioBuffering && a.readyState >= 3) {
              setIsAudioBuffering(false);
            }

            const now = Date.now();
            if (now - lastSaveTimeRef.current > 10000 && a.duration > 0) {
              lastSaveTimeRef.current = now;
              saveFileProgress(file.id, a.currentTime / a.duration).catch(console.error);
            }
          }
        }}
        onLoadedMetadata={() => {
          const a = audioRef.current;
          if (a) {
            setAudioDuration(a.duration || 0);
            if (file.progress && file.progress > 0 && file.progress < 1) {
              a.currentTime = (a.duration || 0) * file.progress;
            }
          }
        }}
        onProgress={() => {
          if (audioRef.current) {
            setAudioBuffered(getBufferedPercent(audioRef.current));
          }
        }}
        onEnded={() => {
          setIsPlayingAudio(false);
          setIsAudioBuffering(false);
        }}
      />

      <div className="audio-card-studio">
        {/* Buffering Overlay */}
        {isAudioBuffering && (
          <div className="audio-buffering-overlay" aria-live="polite">
            <div className="audio-buffering-spinner" />
            <span className="audio-buffering-text">Buffering...</span>
          </div>
        )}

        {/* Vinyl / Lossless Emblem */}
        <div className="audio-hero-disc-wrap">
          <div className={`audio-vinyl-disc ${isPlayingAudio ? 'spinning' : ''}`}>
            <div className="audio-vinyl-groove" />
            <div className="audio-vinyl-groove groove-2" />
            <div className="audio-vinyl-center">
              <Music size={16} className="text-primary" />
            </div>
          </div>
          <div className="audio-lossless-pill">
            <span className="lossless-pulse-dot" />
            <span>24-BIT / 96kHz LOSSLESS</span>
            <span className="lossless-divider">•</span>
            <span>AES-256-GCM</span>
          </div>
        </div>

        {/* Animated Cinema OS Equalizer Waveform */}
        <div className="audio-visual-wave-container">
          <div className="audio-visual-wave">
            {Array.from({ length: 36 }).map((_, i) => {
              const bellCurve = Math.sin((i / 35) * Math.PI);
              const dynamicHeight = isPlayingAudio
                ? Math.max(12, Math.sin(i * 0.45 + audioProgress * 4) * 26 + 32 * bellCurve)
                : Math.max(8, 14 * bellCurve + (i % 3) * 3);
              return (
                <div
                  key={i}
                  className={`wave-bar ${isPlayingAudio ? 'animating' : ''}`}
                  style={{
                    height: `${dynamicHeight}px`,
                    animationDelay: `${(i % 12) * 0.08}s`,
                  }}
                />
              );
            })}
          </div>
          {/* Mirror Reflection beneath baseline */}
          <div className="audio-visual-wave reflection" aria-hidden="true">
            {Array.from({ length: 36 }).map((_, i) => {
              const bellCurve = Math.sin((i / 35) * Math.PI);
              const dynamicHeight = isPlayingAudio
                ? Math.max(5, (Math.sin(i * 0.45 + audioProgress * 4) * 26 + 32 * bellCurve) * 0.35)
                : Math.max(3, (14 * bellCurve + (i % 3) * 3) * 0.35);
              return (
                <div
                  key={i}
                  className={`wave-bar ${isPlayingAudio ? 'animating' : ''}`}
                  style={{
                    height: `${dynamicHeight}px`,
                    animationDelay: `${(i % 12) * 0.08}s`,
                  }}
                />
              );
            })}
          </div>
        </div>

        <div className="audio-track-info">
          <h4 className="audio-track-title">{file.name}</h4>
          <p className="audio-track-meta">
            {isAudioBuffering ? (
              <span className="audio-meta-buffering">Buffering stream from Telegram...</span>
            ) : (
              `${formatFileSize(file.size)} • AES-256 Decrypted Telegram Stream`
            )}
          </p>
        </div>

        {/* Scrubber with buffer bar */}
        <div className="audio-scrub-row">
          <span className="audio-time-label">{formatTime(audioProgress)}</span>
          <div className="audio-progress-container">
            <div className="audio-buffer-bar" style={{ width: `${audioBuffered}%` }} />
            <input
              type="range"
              min={0}
              max={audioDuration || 100}
              step={0.1}
              value={audioProgress}
              onChange={handleAudioSeek}
              className="audio-seek-slider"
            />
          </div>
          <span className="audio-time-label">{formatTime(audioDuration)}</span>
        </div>

        {/* Controls */}
        <div className="audio-controls-row">
          <button className="audio-play-btn" onClick={togglePlayAudio} title={isPlayingAudio ? 'Pause' : 'Play'}>
            {isAudioBuffering ? (
              <div className="audio-btn-spinner" />
            ) : isPlayingAudio ? (
              <Pause size={24} />
            ) : (
              <Play size={24} className="ml-0-5" />
            )}
          </button>

          <div className="audio-volume-group">
            <button
              className="volume-btn"
              onClick={() => {
                if (audioRef.current) {
                  const newMute = !isAudioMuted;
                  setIsAudioMuted(newMute);
                  audioRef.current.muted = newMute;
                }
              }}
              title={isAudioMuted ? 'Unmute' : 'Mute'}
            >
              {isAudioMuted || audioVolume === 0 ? <VolumeX size={18} /> : <Volume2 size={18} />}
            </button>
            <input
              type="range"
              min={0}
              max={1}
              step={0.05}
              value={isAudioMuted ? 0 : audioVolume}
              onChange={(e) => {
                const v = parseFloat(e.target.value);
                setAudioVolume(v);
                setIsAudioMuted(false);
                if (audioRef.current) {
                  audioRef.current.volume = v;
                  audioRef.current.muted = false;
                }
              }}
              className="volume-slider"
            />
          </div>
        </div>

        {/* Dedicated "Play in Floating Music Player Below" button */}
        {onDockToBottomPlayer && (
          <button
            type="button"
            className="audio-dock-cta-btn"
            onClick={() => {
              onDockToBottomPlayer(file);
              onClose();
            }}
            title="Minimize preview and stream through the persistent bottom music player"
          >
            <Music size={16} className="text-primary" />
            <span>Play in Floating Music Player Below</span>
            <ExternalLink size={14} className="ml-auto opacity-70" />
          </button>
        )}
      </div>
    </div>
  );
};
