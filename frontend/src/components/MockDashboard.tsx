import React from 'react';
import {
  Home,
  Folder,
  Clock,
  Star,
  Film,
  Music,
  Image as ImageIcon,
  FileText,
  Code2,
  Archive,
  ArrowDownUp,
  Trash2,
  Settings,
  Search,
  Upload,
  Play,
  Pause,
  SkipBack,
  SkipForward,
  Volume2,
  Maximize2,
  X
} from 'lucide-react';
import '../styles/mockup.css';

export default function MockDashboard() {
  return (
    <div className="mock-app-container">
      {/* SIDEBAR */}
      <aside className="mock-sidebar">
        <div className="mock-brand">
          <div className="mock-brand-icon">
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
              <rect width="24" height="24" rx="6" fill="#2563eb" />
              <path d="M7 12L12 7L17 12L12 17L7 12Z" fill="white" />
            </svg>
          </div>
          <span className="mock-brand-name">TgDrive</span>
        </div>

        <nav className="mock-nav-group">
          <a href="#mockup" className="mock-nav-item active">
            <Home size={17} />
            <span>Home</span>
          </a>
          <a href="#mockup" className="mock-nav-item">
            <Folder size={17} />
            <span>All Files</span>
          </a>
          <a href="#mockup" className="mock-nav-item">
            <Clock size={17} />
            <span>Recent</span>
          </a>
          <a href="#mockup" className="mock-nav-item">
            <Star size={17} />
            <span>Starred</span>
          </a>
        </nav>

        <div className="mock-nav-label">LIBRARY</div>
        <nav className="mock-nav-group">
          <a href="#mockup" className="mock-nav-item">
            <Film size={17} />
            <span>Videos</span>
          </a>
          <a href="#mockup" className="mock-nav-item">
            <Music size={17} />
            <span>Music</span>
          </a>
          <a href="#mockup" className="mock-nav-item">
            <ImageIcon size={17} />
            <span>Photos</span>
          </a>
          <a href="#mockup" className="mock-nav-item">
            <FileText size={17} />
            <span>Documents</span>
          </a>
          <a href="#mockup" className="mock-nav-item">
            <Code2 size={17} />
            <span>Code</span>
          </a>
          <a href="#mockup" className="mock-nav-item">
            <Archive size={17} />
            <span>Archives</span>
          </a>
        </nav>

        <div className="mock-nav-label">SYSTEM</div>
        <nav className="mock-nav-group">
          <a href="#mockup" className="mock-nav-item">
            <ArrowDownUp size={17} />
            <span>Transfers</span>
          </a>
          <a href="#mockup" className="mock-nav-item">
            <Trash2 size={17} />
            <span>Trash</span>
          </a>
          <a href="#mockup" className="mock-nav-item">
            <Settings size={17} />
            <span>Settings</span>
          </a>
        </nav>

        <div className="mock-storage-widget">
          <div className="mock-storage-header">412.6 GB stored · 18,402 files</div>
          <div className="mock-storage-bar-mini">
            <div className="mock-bar-mini-seg" style={{ width: '45%', background: '#3b82f6' }}></div>
            <div className="mock-bar-mini-seg" style={{ width: '18%', background: '#06b6d4' }}></div>
            <div className="mock-bar-mini-seg" style={{ width: '12%', background: '#10b981' }}></div>
            <div className="mock-bar-mini-seg" style={{ width: '10%', background: '#f97316' }}></div>
            <div className="mock-bar-mini-seg" style={{ width: '8%', background: '#ef4444' }}></div>
            <div className="mock-bar-mini-seg" style={{ width: '7%', background: '#eab308' }}></div>
          </div>
          <div className="mock-storage-status">
            <span className="mock-status-dot"></span>
            Cloud connected · Encrypted
          </div>
        </div>
      </aside>

      {/* MAIN CONTENT */}
      <main className="mock-main">
        {/* TOP BAR */}
        <header className="mock-topbar">
          <div className="mock-topbar-left">
            <span className="mock-breadcrumb">Home</span>
          </div>

          <div className="mock-topbar-center">
            <div className="mock-search-bar">
              <Search size={15} className="mock-search-icon" />
              <input type="text" placeholder="Search files, videos, music, code..." readOnly />
              <div className="mock-search-shortcut">⌘K</div>
            </div>
          </div>

          <div className="mock-topbar-actions">
            <button className="mock-btn-upload">
              <Upload size={15} />
              <span>Upload</span>
            </button>
            <button className="mock-btn-close">
              <X size={16} />
            </button>
          </div>
        </header>

        {/* DASHBOARD CONTENT */}
        <div className="mock-dashboard-scroll">
          <div className="mock-dashboard-inner">
            <div className="mock-greeting">
              <h1>Good evening, Jack</h1>
              <p>Pick up where you left off. 3 transfers running in the background.</p>
            </div>

            {/* CONTINUE WATCHING */}
            <section className="mock-section">
              <div className="mock-section-header">
                <h2>Continue Watching</h2>
                <a href="#mockup" className="mock-link">View all videos</a>
              </div>
              <div className="mock-continue-grid">
                {/* Video 1 */}
                <div className="mock-video-card">
                  <div className="mock-video-thumb" style={{ backgroundImage: 'url(https://images.unsplash.com/photo-1506703719100-a0f3a48c0f86?w=800&auto=format&fit=crop&q=80)' }}>
                    <button className="mock-play-btn"><Play size={16} fill="currentColor" /></button>
                    <span className="mock-video-badge">4K · HDR</span>
                  </div>
                  <div className="mock-video-progress">
                    <div className="mock-video-progress-fill" style={{ width: '35%' }}></div>
                  </div>
                  <div className="mock-video-info">
                    <h3>Interstellar.mkv</h3>
                    <span>01:42:16 / 02:49:00</span>
                  </div>
                </div>

                {/* Video 2 */}
                <div className="mock-video-card">
                  <div className="mock-video-thumb" style={{ backgroundImage: 'url(https://images.unsplash.com/photo-1509316975850-ff9c5deb0cd9?w=800&auto=format&fit=crop&q=80)' }}>
                    <button className="mock-play-btn"><Play size={16} fill="currentColor" /></button>
                    <span className="mock-video-badge">1080p</span>
                  </div>
                  <div className="mock-video-progress">
                    <div className="mock-video-progress-fill" style={{ width: '15%' }}></div>
                  </div>
                  <div className="mock-video-info">
                    <h3>Dune Part Two.mp4</h3>
                    <span>00:38:02 / 02:46:00</span>
                  </div>
                </div>

                {/* Video 3 */}
                <div className="mock-video-card">
                  <div className="mock-video-thumb" style={{ backgroundImage: 'url(https://images.unsplash.com/photo-1464822759023-fed622ff2c3b?w=800&auto=format&fit=crop&q=80)' }}>
                    <button className="mock-play-btn"><Play size={16} fill="currentColor" /></button>
                    <span className="mock-video-badge">4K</span>
                  </div>
                  <div className="mock-video-progress">
                    <div className="mock-video-progress-fill" style={{ width: '60%' }}></div>
                  </div>
                  <div className="mock-video-info">
                    <h3>Road Trip 2026.mov</h3>
                    <span>00:12:40 / 00:18:05</span>
                  </div>
                </div>
              </div>
            </section>

            {/* LOWER SECTION: RECENTLY OPENED & STORAGE */}
            <div className="mock-bottom-grid">
              {/* Recently Opened */}
              <section className="mock-section mock-recent-section">
                <h2>Recently Opened</h2>
                <div className="mock-recent-list">
                  <div className="mock-recent-item">
                    <div className="mock-recent-icon video"><Film size={17} /></div>
                    <div className="mock-recent-meta">
                      <h4>Interstellar.mkv</h4>
                      <span>Videos / Films</span>
                    </div>
                    <div className="mock-recent-time">Watched 2h ago</div>
                    <div className="mock-recent-size">18.4 GB</div>
                  </div>

                  <div className="mock-recent-item">
                    <div className="mock-recent-icon music"><Music size={17} /></div>
                    <div className="mock-recent-meta">
                      <h4>Weightless.flac</h4>
                      <span>Music / Ambient</span>
                    </div>
                    <div className="mock-recent-time">Played 3h ago</div>
                    <div className="mock-recent-size">42.1 MB</div>
                  </div>

                  <div className="mock-recent-item">
                    <div className="mock-recent-icon document"><FileText size={17} /></div>
                    <div className="mock-recent-meta">
                      <h4>Q3 Report.pdf</h4>
                      <span>Documents / Work</span>
                    </div>
                    <div className="mock-recent-time">Page 14 of 38</div>
                    <div className="mock-recent-size">3.2 MB</div>
                  </div>

                  <div className="mock-recent-item">
                    <div className="mock-recent-icon code"><Code2 size={17} /></div>
                    <div className="mock-recent-meta">
                      <h4>main.py</h4>
                      <span>Code / tgdrive-api</span>
                    </div>
                    <div className="mock-recent-time">Edited yesterday</div>
                    <div className="mock-recent-size">12 KB</div>
                  </div>

                  <div className="mock-recent-item">
                    <div className="mock-recent-icon archive"><Archive size={17} /></div>
                    <div className="mock-recent-meta">
                      <h4>project.zip</h4>
                      <span>Archives</span>
                    </div>
                    <div className="mock-recent-time">214 files inside</div>
                    <div className="mock-recent-size">1.1 GB</div>
                  </div>

                  <div className="mock-recent-item">
                    <div className="mock-recent-icon photo"><ImageIcon size={17} /></div>
                    <div className="mock-recent-meta">
                      <h4>vacation-041.jpg</h4>
                      <span>Photos / Lisbon</span>
                    </div>
                    <div className="mock-recent-time">Viewed Mon</div>
                    <div className="mock-recent-size">6.8 MB</div>
                  </div>
                </div>
              </section>

              {/* Storage */}
              <section className="mock-section mock-storage-section">
                <h2>Storage</h2>
                <div className="mock-storage-card">
                  <div className="mock-storage-big">
                    <span className="mock-storage-amount">412.6</span>
                    <span className="mock-storage-unit">GB</span>
                  </div>

                  <div className="mock-storage-multi-bar">
                    <div className="mock-bar-segment" style={{ width: '45%', background: '#3b82f6' }}></div>
                    <div className="mock-bar-segment" style={{ width: '18%', background: '#06b6d4' }}></div>
                    <div className="mock-bar-segment" style={{ width: '12%', background: '#10b981' }}></div>
                    <div className="mock-bar-segment" style={{ width: '10%', background: '#f97316' }}></div>
                    <div className="mock-bar-segment" style={{ width: '8%', background: '#ef4444' }}></div>
                    <div className="mock-bar-segment" style={{ width: '7%', background: '#eab308' }}></div>
                  </div>

                  <div className="mock-storage-legend">
                    <div className="mock-legend-item"><span className="mock-dot" style={{ background: '#3b82f6' }}></span> Video <span className="mock-legend-size">189.8 GB</span></div>
                    <div className="mock-legend-item"><span className="mock-dot" style={{ background: '#06b6d4' }}></span> Photos <span className="mock-legend-size">74.3 GB</span></div>
                    <div className="mock-legend-item"><span className="mock-dot" style={{ background: '#10b981' }}></span> Audio <span className="mock-legend-size">49.5 GB</span></div>
                    <div className="mock-legend-item"><span className="mock-dot" style={{ background: '#f97316' }}></span> Archives <span className="mock-legend-size">45.4 GB</span></div>
                    <div className="mock-legend-item"><span className="mock-dot" style={{ background: '#ef4444' }}></span> Documents <span className="mock-legend-size">33.0 GB</span></div>
                    <div className="mock-legend-item"><span className="mock-dot" style={{ background: '#eab308' }}></span> Code & other <span className="mock-legend-size">20.6 GB</span></div>
                  </div>

                  <div className="mock-storage-footer">
                    <span>18,402 files</span>
                    <span>1,206 folders</span>
                    <span className="mock-encryption-badge">AES-256</span>
                  </div>
                </div>
              </section>
            </div>
          </div>
        </div>
      </main>

      {/* BOTTOM PLAYER */}
      <footer className="mock-player">
        <div className="mock-player-track">
          <div className="mock-track-art"></div>
          <div className="mock-track-info">
            <h4>Weightless</h4>
            <span>Marconi Union · FLAC 24-bit</span>
          </div>
        </div>

        <div className="mock-player-controls-wrapper">
          <div className="mock-player-controls">
            <button className="mock-ctrl-btn" aria-label="Previous"><SkipBack size={17} /></button>
            <button className="mock-play-pause-btn" aria-label="Pause"><Pause size={17} fill="currentColor" /></button>
            <button className="mock-ctrl-btn" aria-label="Next"><SkipForward size={17} /></button>
          </div>
          <div className="mock-player-scrubber">
            <span>03:12</span>
            <div className="mock-scrubber-track">
              <div className="mock-scrubber-fill" style={{ width: '38%' }}>
                <div className="mock-scrubber-thumb"></div>
              </div>
            </div>
            <span>08:09</span>
          </div>
        </div>

        <div className="mock-player-actions">
          <div className="mock-upload-status">
            <ArrowDownUp size={13} className="mock-upload-icon" />
            <span>Uploading 4 files · 62%</span>
          </div>
          <button className="mock-ctrl-btn" aria-label="Volume"><Volume2 size={17} /></button>
          <button className="mock-ctrl-btn" aria-label="Fullscreen"><Maximize2 size={17} /></button>
        </div>
      </footer>
    </div>
  );
}
