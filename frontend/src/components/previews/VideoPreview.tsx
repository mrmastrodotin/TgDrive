import React, { useState, useEffect, useRef, useCallback } from 'react';
import {
  Play,
  Pause,
  SkipBack,
  SkipForward,
  Volume2,
  VolumeX,
  Maximize,
  Minimize,
  Info,
  Tv,
  PictureInPicture2,
  Film,
  Subtitles,
} from 'lucide-react';
import { FileItem, formatFileSize } from '../../types';
import { getScopedStreamURL, warmupFile, getWarmupStatus, saveFileProgress } from '../../api/client';
import { formatTime } from './utils';

export interface VideoPreviewProps {
  file: FileItem;
  isPiPMode: boolean;
  isPiPActive: boolean;
  isSystemPiPActive: boolean;
  onTogglePiP: () => void;
  isTheaterMode?: boolean;
  onToggleTheaterMode?: () => void;
  isControlsVisible?: boolean;
  onResetControlsTimer?: (delay?: number) => void;
  isPiPControlsVisible?: boolean;
  onResetPiPControlsTimer?: (delay?: number) => void;
}

export const VideoPreview: React.FC<VideoPreviewProps> = ({
  file,
  isPiPMode,
  isPiPActive,
  isSystemPiPActive,
  onTogglePiP,
  isTheaterMode: propTheaterMode,
  onToggleTheaterMode,
  isControlsVisible,
  onResetControlsTimer,
  isPiPControlsVisible,
  onResetPiPControlsTimer,
}) => {
  const videoRef = useRef<HTMLVideoElement>(null);
  const videoContainerRef = useRef<HTMLDivElement>(null);

  const [localTheaterMode, setLocalTheaterMode] = useState(false);
  const isTheaterMode = propTheaterMode !== undefined ? propTheaterMode : localTheaterMode;
  const toggleTheaterMode = useCallback(() => {
    if (onToggleTheaterMode) {
      onToggleTheaterMode();
    } else {
      setLocalTheaterMode((prev) => !prev);
    }
  }, [onToggleTheaterMode]);

  const [videoPlaybackRate, setVideoPlaybackRate] = useState(1);
  const [isVideoPlaying, setIsVideoPlaying] = useState(false);
  const [isVideoBuffering, setIsVideoBuffering] = useState(false);
  const [videoCurrentTime, setVideoCurrentTime] = useState(0);
  const [videoDuration, setVideoDuration] = useState(0);
  const [videoBuffered, setVideoBuffered] = useState(0);
  const [videoBufferedRanges, setVideoBufferedRanges] = useState<{ start: number; end: number }[]>([]);
  const [videoVolume, setVideoVolume] = useState(1);
  const [isVideoMuted, setIsVideoMuted] = useState(false);
  const [isVideoFullscreen, setIsVideoFullscreen] = useState(false);
  const [showVideoControls, setShowVideoControls] = useState(true);
  const effectiveControlsProp = isControlsVisible ?? isPiPControlsVisible;
  const resetTimerProp = onResetControlsTimer ?? onResetPiPControlsTimer;
  const effectiveControlsVisible = isVideoFullscreen ? showVideoControls : (effectiveControlsProp !== undefined ? effectiveControlsProp : showVideoControls);
  const [showStats, setShowStats] = useState(false);
  const [scrubberHover, setScrubberHover] = useState<{ time: number; percent: number } | null>(null);
  const [skipFlash, setSkipFlash] = useState<{ side: 'left' | 'right'; text: string } | null>(null);
  const [videoSrc, setVideoSrc] = useState<string>('');

  useEffect(() => {
    let cancelled = false;
    getScopedStreamURL(file.id)
      .then((url) => {
        if (!cancelled) setVideoSrc(url);
      })
      .catch((err) => console.warn('Failed to load scoped stream URL:', err));
    return () => {
      cancelled = true;
    };
  }, [file.id]);

  const skipFlashTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const videoControlsTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastSaveTimeRef = useRef<number>(0);

  const [subtitleUrl, setSubtitleUrl] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const handleSubtitleUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0];
    if (!f) return;
    const reader = new FileReader();
    reader.onload = (evt) => {
      let text = evt.target?.result as string;
      if (f.name.toLowerCase().endsWith('.srt')) {
        text = 'WEBVTT\n\n' + text.replace(/(\d{2}:\d{2}:\d{2}),(\d{3})/g, '$1.$2');
      }
      const blob = new Blob([text], { type: 'text/vtt' });
      const url = URL.createObjectURL(blob);
      setSubtitleUrl(url);
    };
    reader.readAsText(f);
  };

  // Multi-chunk warmup detection (based on file's actual chunk size, defaulting to 16MB)
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
        console.warn('Warmup failed, loading directly:', err);
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

  // Buffer range calculations
  const updateVideoBufferInfo = useCallback((v: HTMLVideoElement) => {
    if (!v.duration || !v.buffered || v.buffered.length === 0) return;
    try {
      const ranges: { start: number; end: number }[] = [];
      for (let i = 0; i < v.buffered.length; i++) {
        const s = (v.buffered.start(i) / v.duration) * 100;
        const e = (v.buffered.end(i) / v.duration) * 100;
        ranges.push({ start: s, end: e });
      }
      setVideoBufferedRanges(ranges);

      const lastEnd = v.buffered.end(v.buffered.length - 1);
      setVideoBuffered(Math.min(100, (lastEnd / v.duration) * 100));
    } catch {
      // Ignore buffer calc errors
    }
  }, []);

  // Auto-hide video controls (YouTube/Netflix style: 3s inactivity, 800ms mouseleave)
  const resetVideoControlsTimer = useCallback((delay = 3000) => {
    if (resetTimerProp) {
      resetTimerProp(delay);
    }
    setShowVideoControls(true);
    if (videoControlsTimerRef.current) clearTimeout(videoControlsTimerRef.current);
    videoControlsTimerRef.current = setTimeout(() => {
      setShowVideoControls(false);
    }, delay);
  }, [resetTimerProp]);

  const handleVideoMouseLeave = useCallback(() => {
    if (resetTimerProp) {
      resetTimerProp(800);
    }
    if (videoControlsTimerRef.current) clearTimeout(videoControlsTimerRef.current);
    videoControlsTimerRef.current = setTimeout(() => {
      setShowVideoControls(false);
    }, 800);
  }, [resetTimerProp]);

  useEffect(() => {
    resetVideoControlsTimer(3000);
    return () => {
      if (videoControlsTimerRef.current) clearTimeout(videoControlsTimerRef.current);
    };
  }, [isPiPMode, resetVideoControlsTimer]);

  // Playback control helpers
  const toggleVideoPlay = useCallback(() => {
    const v = videoRef.current;
    if (!v) return;
    if (v.paused) {
      v.play().catch(() => {});
      setIsVideoPlaying(true);
      resetVideoControlsTimer(2500);
    } else {
      v.pause();
      setIsVideoPlaying(false);
      setShowVideoControls(true);
      if (isPiPMode) {
        resetVideoControlsTimer(2500);
      }
    }
  }, [isPiPMode, resetVideoControlsTimer]);

  const triggerSkipFlash = useCallback((side: 'left' | 'right', text: string) => {
    setSkipFlash({ side, text });
    if (skipFlashTimerRef.current) clearTimeout(skipFlashTimerRef.current);
    skipFlashTimerRef.current = setTimeout(() => setSkipFlash(null), 650);
  }, []);

  const videoSeekBy = useCallback((sec: number) => {
    const v = videoRef.current;
    if (!v) return;
    setIsVideoBuffering(true);
    v.currentTime = Math.max(0, Math.min(v.duration || 0, v.currentTime + sec));
    triggerSkipFlash(sec > 0 ? 'right' : 'left', `${Math.abs(sec)}s`);
  }, [triggerSkipFlash]);

  const handlePlaybackSpeed = (speed: number) => {
    setVideoPlaybackRate(speed);
    if (videoRef.current) {
      videoRef.current.playbackRate = speed;
    }
  };

  const toggleVideoFullscreen = useCallback(async () => {
    const c = videoContainerRef.current;
    if (!c) return;
    if (!document.fullscreenElement) {
      await c.requestFullscreen().catch(() => {});
    } else {
      await document.exitFullscreen().catch(() => {});
    }
  }, []);

  // Keyboard controls
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const v = videoRef.current;
      if (!v) return;
      if ((e.target as HTMLElement).tagName === 'INPUT') return;

      if (e.key >= '0' && e.key <= '9') {
        e.preventDefault();
        const pct = parseInt(e.key, 10) / 10;
        setIsVideoBuffering(true);
        v.currentTime = pct * (v.duration || 0);
        triggerSkipFlash(pct > (v.currentTime / (v.duration || 1)) ? 'right' : 'left', `${pct * 100}%`);
        return;
      }

      switch (e.key) {
        case ' ':
        case 'k':
        case 'K':
          e.preventDefault();
          toggleVideoPlay();
          break;
        case 'ArrowLeft':
        case 'j':
        case 'J':
          e.preventDefault();
          videoSeekBy(-10);
          break;
        case 'ArrowRight':
        case 'l':
        case 'L':
          e.preventDefault();
          videoSeekBy(10);
          break;
        case 'ArrowUp':
          e.preventDefault();
          v.volume = Math.min(1, v.volume + 0.1);
          setVideoVolume(v.volume);
          break;
        case 'ArrowDown':
          e.preventDefault();
          v.volume = Math.max(0, v.volume - 0.1);
          setVideoVolume(v.volume);
          break;
        case 'f':
        case 'F':
          e.preventDefault();
          toggleVideoFullscreen();
          break;
        case 't':
        case 'T':
          e.preventDefault();
          toggleTheaterMode();
          break;
        case 'i':
        case 'I':
        case 'p':
        case 'P':
          e.preventDefault();
          onTogglePiP();
          break;
        case 'm':
        case 'M':
          e.preventDefault();
          v.muted = !v.muted;
          setIsVideoMuted(v.muted);
          break;
        case ',':
          e.preventDefault();
          v.currentTime = Math.max(0, v.currentTime - 1 / 30);
          break;
        case '.':
          e.preventDefault();
          v.currentTime = Math.min(v.duration || 0, v.currentTime + 1 / 30);
          break;
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [toggleVideoPlay, videoSeekBy, toggleVideoFullscreen, toggleTheaterMode, onTogglePiP, triggerSkipFlash]);

  // Fullscreen change listener
  useEffect(() => {
    const handler = () => setIsVideoFullscreen(!!document.fullscreenElement);
    document.addEventListener('fullscreenchange', handler);
    return () => document.removeEventListener('fullscreenchange', handler);
  }, []);

  if (isWarming && !warmupReady) {
    return (
      <div className="preview-video-viewport">
        <div className="video-warmup-overlay">
          <div className="warmup-spinner" />
          <p className="warmup-text">{warmupProgress || 'Preparing stream...'}</p>
          <p className="warmup-subtext">Decrypting chunks from Telegram CDN</p>
        </div>
      </div>
    );
  }

  return (
    <div className={`preview-video-viewport ${isTheaterMode ? 'theater' : ''}`}>
      <div
        ref={videoContainerRef}
        className={`video-player-container ${isVideoFullscreen ? 'fullscreen' : ''} ${isTheaterMode ? 'theater' : ''} ${isPiPMode ? 'pip-player' : ''} ${!effectiveControlsVisible ? 'controls-hidden' : ''}`}
        onMouseMove={() => resetVideoControlsTimer(3000)}
        onMouseEnter={() => resetVideoControlsTimer(3000)}
        onMouseLeave={handleVideoMouseLeave}
      >
        {/* System-Wide PiP active placeholder banner removed to prevent secondary window */}

        {/* Fullscreen top header overlay (movie title + badges + PiP + exit fullscreen button) */}
        {isVideoFullscreen && (
          <div className={`video-fullscreen-top-bar ${effectiveControlsVisible ? 'visible' : 'hidden'}`}>
            <div className="video-fullscreen-left">
              <div className="type-icon-badge">
                <Film size={18} />
              </div>
              <div className="video-fullscreen-title-wrap">
                <div className="title-flex-row">
                  <h3 className="video-fullscreen-title" title={file.name}>{file.name}</h3>
                  <span className="canvas-type-tag">Cinematic Video</span>
                </div>
                <div className="title-meta-pills">
                  <span className="meta-pill">{formatFileSize(file.size)}</span>
                  <span className="meta-pill cipher-pill">AES-256-GCM</span>
                </div>
              </div>
            </div>
            <div className="video-fullscreen-controls">
              <button
                className="icon-action-btn pip-btn"
                onClick={onTogglePiP}
                title="Picture-in-Picture (P)"
              >
                <PictureInPicture2 size={17} />
              </button>
              <button
                className="icon-action-btn fullscreen-exit-btn"
                onClick={toggleVideoFullscreen}
                title="Exit Fullscreen (F / Esc)"
              >
                <Minimize size={18} />
              </button>
            </div>
          </div>
        )}

        <video
          ref={videoRef}
          playsInline
          className="preview-video-element"
          src={videoSrc || undefined}
          onError={(e) => {
            console.error('Video stream playback error:', e);
            setIsVideoBuffering(false);
          }}
          onPlay={() => {
            setIsVideoPlaying(true);
            setIsVideoBuffering(false);
            resetVideoControlsTimer(3000);
          }}
          onPause={() => {
            setIsVideoPlaying(false);
            setShowVideoControls(true);
            if (resetTimerProp) resetTimerProp(3000);
            if (isPiPMode) {
              resetVideoControlsTimer(3000);
            }
            const v = videoRef.current;
            if (v && v.duration > 0) {
              lastSaveTimeRef.current = Date.now();
              saveFileProgress(file.id, v.currentTime / v.duration).catch(console.error);
            }
          }}
          onSeeking={() => setIsVideoBuffering(true)}
          onSeeked={() => {
            const v = videoRef.current;
            if (v && v.readyState >= 3) {
              setIsVideoBuffering(false);
            }
            resetVideoControlsTimer(3000);
          }}
          onWaiting={() => setIsVideoBuffering(true)}
          onStalled={() => {
            const v = videoRef.current;
            if (v && !v.paused) setIsVideoBuffering(true);
          }}
          onPlaying={() => {
            setIsVideoPlaying(true);
            setIsVideoBuffering(false);
            resetVideoControlsTimer(3000);
          }}

          onCanPlay={() => {
            if (videoRef.current && !videoRef.current.paused) setIsVideoPlaying(true);
            setIsVideoBuffering(false);
          }}
          onCanPlayThrough={() => setIsVideoBuffering(false)}
          onTimeUpdate={() => {
            const v = videoRef.current;
            if (v) {
              setVideoCurrentTime(v.currentTime);
              setVideoDuration(v.duration || 0);
              updateVideoBufferInfo(v);
              if (isVideoBuffering && v.readyState >= 3) {
                setIsVideoBuffering(false);
              }
              
              const now = Date.now();
              if (now - lastSaveTimeRef.current > 10000 && v.duration > 0) {
                lastSaveTimeRef.current = now;
                saveFileProgress(file.id, v.currentTime / v.duration).catch(console.error);
              }
            }
          }}
          onLoadedMetadata={() => {
            const v = videoRef.current;
            if (v) {
              setVideoDuration(v.duration || 0);
              if (file.progress && file.progress > 0 && file.progress < 1) {
                v.currentTime = (v.duration || 0) * file.progress;
              }
              v.play().catch(() => {});
            }
          }}
          onProgress={() => {
            const v = videoRef.current;
            if (v) updateVideoBufferInfo(v);
          }}
        >
          {subtitleUrl && <track kind="subtitles" src={subtitleUrl} srcLang="en" label="External Subtitle" default />}
        </video>

        {/* Buffering spinner */}
        {isVideoBuffering && (
          <div className="video-buffering-overlay" aria-live="polite">
            <div className="video-buffering-spinner" />
            <span className="video-buffering-text">Buffering...</span>
          </div>
        )}

        {/* Gesture overlay */}
        <div
          className="video-gesture-overlay"
          onClick={() => {
            if (!effectiveControlsVisible) {
              resetVideoControlsTimer(3000);
            } else {
              toggleVideoPlay();
            }
          }}
        >
          <div
            className="video-gesture-side left"
            onDoubleClick={(e) => {
              e.stopPropagation();
              videoSeekBy(-10);
            }}
          />
          <div
            className="video-gesture-side right"
            onDoubleClick={(e) => {
              e.stopPropagation();
              videoSeekBy(10);
            }}
          />
        </div>

        {/* Skip flash ripple */}
        {skipFlash && (
          <div className={`video-skip-flash-badge ${skipFlash.side}`}>
            <div className="skip-flash-inner">
              <span>{skipFlash.text}</span>
            </div>
          </div>
        )}

        {/* Stats for Nerds */}
        {showStats && (
          <div className="video-stats-card">
            <div className="stats-card-header">
              <span className="stats-title">Stats for Nerds</span>
              <button onClick={() => setShowStats(false)} className="stats-card-close">
                ×
              </button>
            </div>
            <div className="stats-card-body">
              <div className="stats-row">
                <span className="stats-label">Resolution</span>
                <span className="stats-val">
                  {videoRef.current ? `${videoRef.current.videoWidth} × ${videoRef.current.videoHeight}` : 'Loading'}
                </span>
              </div>
              <div className="stats-row">
                <span className="stats-label">Buffer Health</span>
                <span className="stats-val stats-highlight">
                  {videoRef.current && videoDuration
                    ? `${Math.max(0, (videoBuffered / 100) * videoDuration - videoCurrentTime).toFixed(1)}s ahead`
                    : '0.0s'}
                </span>
              </div>
              <div className="stats-row">
                <span className="stats-label">Prefetch Engine</span>
                <span className="stats-val stats-highlight">Active (+2 Chunks ~40MB)</span>
              </div>
              <div className="stats-row">
                <span className="stats-label">Speed</span>
                <span className="stats-val">{videoPlaybackRate}x</span>
              </div>
              <div className="stats-row">
                <span className="stats-label">Encryption</span>
                <span className="stats-val">AES-256-GCM Streaming</span>
              </div>
            </div>
          </div>
        )}

        {/* Controls overlay */}
        <div className={`video-controls-overlay ${effectiveControlsVisible ? 'visible' : 'hidden'}`}>
          {/* Scrubber */}
          <div
            className="video-progress-bar-container"
            onMouseMove={(e) => {
              const v = videoRef.current;
              if (!v || !v.duration) return;
              const rect = e.currentTarget.getBoundingClientRect();
              const ratio = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
              setScrubberHover({
                time: ratio * v.duration,
                percent: ratio * 100,
              });
            }}
            onMouseLeave={() => setScrubberHover(null)}
            onClick={(e) => {
              const v = videoRef.current;
              if (!v || !v.duration) return;
              const rect = e.currentTarget.getBoundingClientRect();
              const ratio = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
              setIsVideoBuffering(true);
              v.currentTime = ratio * v.duration;
            }}
          >
            {scrubberHover && (
              <div className="video-scrubber-tooltip" style={{ left: `${scrubberHover.percent}%` }}>
                {formatTime(scrubberHover.time)}
              </div>
            )}
            <div className="video-progress-track" />
            {videoBufferedRanges.length > 0 ? (
              videoBufferedRanges.map((range, idx) => (
                <div
                  key={idx}
                  className="video-buffer-bar"
                  style={{
                    left: `${range.start}%`,
                    width: `${Math.max(0, range.end - range.start)}%`,
                  }}
                />
              ))
            ) : (
              <div className="video-buffer-bar" style={{ left: 0, width: `${videoBuffered}%` }} />
            )}
            <div
              className="video-played-bar"
              style={{ width: videoDuration ? `${(videoCurrentTime / videoDuration) * 100}%` : '0%' }}
            />
            <div
              className="video-scrubber-dot"
              style={{ left: videoDuration ? `${(videoCurrentTime / videoDuration) * 100}%` : '0%' }}
            />
          </div>

          {/* Bottom control bar */}
          <div className="video-controls-bottom">
            <div className="video-controls-left">
              <button className="video-ctrl-btn" onClick={toggleVideoPlay} title={isVideoPlaying ? 'Pause (K)' : 'Play (K)'}>
                {isVideoPlaying ? <Pause size={isPiPMode ? 17 : 20} /> : <Play size={isPiPMode ? 17 : 20} />}
              </button>
              <button className="video-ctrl-btn" onClick={() => videoSeekBy(-10)} title="Rewind 10s (J)">
                <SkipBack size={isPiPMode ? 15 : 18} />
              </button>
              <button className="video-ctrl-btn" onClick={() => videoSeekBy(10)} title="Forward 10s (L)">
                <SkipForward size={isPiPMode ? 15 : 18} />
              </button>

              {/* Volume */}
              <button
                className="video-ctrl-btn"
                onClick={() => {
                  const v = videoRef.current;
                  if (v) {
                    v.muted = !v.muted;
                    setIsVideoMuted(v.muted);
                  }
                }}
                title={isVideoMuted ? 'Unmute (M)' : 'Mute (M)'}
              >
                {isVideoMuted || videoVolume === 0 ? <VolumeX size={isPiPMode ? 16 : 18} /> : <Volume2 size={isPiPMode ? 16 : 18} />}
              </button>
              {!isPiPMode && (
                <input
                  type="range"
                  min={0}
                  max={1}
                  step={0.05}
                  value={isVideoMuted ? 0 : videoVolume}
                  onChange={(e) => {
                    const vol = parseFloat(e.target.value);
                    setVideoVolume(vol);
                    setIsVideoMuted(false);
                    if (videoRef.current) {
                      videoRef.current.volume = vol;
                      videoRef.current.muted = false;
                    }
                  }}
                  className="video-volume-slider"
                />
              )}

              <span className="video-time-display">
                {formatTime(videoCurrentTime)} / {formatTime(videoDuration)}
              </span>
            </div>

            <div className="video-controls-right">
              {/* Subtitles */}
              <input 
                type="file" 
                accept=".srt,.vtt" 
                ref={fileInputRef} 
                style={{ display: 'none' }} 
                onChange={handleSubtitleUpload} 
              />
              {!isPiPMode && (
                <button
                  className={`video-ctrl-btn ${subtitleUrl ? 'active' : ''}`}
                  onClick={() => fileInputRef.current?.click()}
                  title="Load Subtitle (.srt / .vtt)"
                >
                  <Subtitles size={18} />
                </button>
              )}

              {/* Stats for nerds */}
              {!isPiPMode && (
                <button
                  className={`video-ctrl-btn stats-btn ${showStats ? 'active' : ''}`}
                  onClick={() => setShowStats((prev) => !prev)}
                  title="Stats for Nerds"
                >
                  <Info size={17} />
                </button>
              )}

              {/* Speed pills */}
              {!isPiPMode && (
                <div className="video-speed-bar">
                  {[0.5, 0.75, 1, 1.25, 1.5, 2].map((s) => (
                    <button
                      key={s}
                      className={`speed-pill ${videoPlaybackRate === s ? 'active' : ''}`}
                      onClick={() => handlePlaybackSpeed(s)}
                    >
                      {s}x
                    </button>
                  ))}
                </div>
              )}

              {/* Theater mode */}
              {!isPiPMode && (
                <button
                  className={`video-ctrl-btn theater-btn ${isTheaterMode ? 'active' : ''}`}
                  onClick={toggleTheaterMode}
                  title={isTheaterMode ? 'Exit Theater Mode (T)' : 'Theater Mode (T)'}
                >
                  <Tv size={18} />
                </button>
              )}

              {/* Fullscreen */}
              <button className="video-ctrl-btn fullscreen-btn" onClick={toggleVideoFullscreen} title="Fullscreen (F)">
                {isVideoFullscreen ? <Minimize size={isPiPMode ? 16 : 18} /> : <Maximize size={isPiPMode ? 16 : 18} />}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
