export interface FileItem {
  id: string;
  name: string;
  size: number;
  mime_type: string;
  folder_id: string | null;
  encryption_salt: string;
  chunk_count: number;
  created_at: string;
}

export interface FolderItem {
  id: string;
  name: string;
  parent_id: string | null;
  created_at: string;
  updated_at: string;
}

export interface FolderContents {
  folder: FolderItem | null;
  folders: FolderItem[];
  files: FileItem[];
}

export interface LoginResponse {
  token: string;
  expires_at: number;
}

export type PreviewableType =
  | 'image'
  | 'video'
  | 'audio'
  | 'pdf'
  | 'markdown'
  | 'code'
  | 'csv'
  | 'presentation'
  | 'office'
  | 'text'
  | 'none';

export type FileCategory = 'all' | 'media' | 'docs' | 'code' | 'archives';

export function getFileCategory(mimeType: string, filename: string): FileCategory {
  if (mimeType.startsWith('image/') || mimeType.startsWith('video/') || mimeType.startsWith('audio/')) {
    return 'media';
  }
  if (
    mimeType === 'application/pdf' ||
    mimeType.includes('document') ||
    mimeType.includes('word') ||
    mimeType.includes('sheet') ||
    mimeType.includes('presentation')
  ) {
    return 'docs';
  }
  const ext = '.' + (filename.split('.').pop() || '').toLowerCase();
  const codeExts = [
    '.js', '.ts', '.jsx', '.tsx', '.py', '.go', '.rs', '.c', '.cpp', '.h', '.hpp',
    '.java', '.kt', '.swift', '.rb', '.php', '.cs',
    '.json', '.yaml', '.yml', '.toml', '.xml', '.html', '.css', '.scss',
    '.md', '.sql', '.sh', '.bat', '.ps1'
  ];
  if (codeExts.includes(ext) || mimeType.startsWith('text/')) {
    return 'code';
  }
  const archExts = ['.zip', '.tar', '.gz', '.7z', '.rar', '.bz2', '.iso'];
  if (archExts.includes(ext) || mimeType.includes('zip') || mimeType.includes('archive') || mimeType.includes('compressed')) {
    return 'archives';
  }
  return 'docs';
}

export function getPreviewType(mimeType: string, filename: string): PreviewableType {
  const ext = '.' + (filename.split('.').pop() || '').toLowerCase();

  // Images
  const imgExts = ['.png', '.jpg', '.jpeg', '.gif', '.webp', '.svg', '.bmp', '.ico', '.avif'];
  if (mimeType.startsWith('image/') || imgExts.includes(ext)) return 'image';

  // Videos
  const videoExts = ['.mp4', '.webm', '.mkv', '.mov', '.avi', '.m4v'];
  if (mimeType.startsWith('video/') || videoExts.includes(ext)) return 'video';

  // Audio
  const audioExts = ['.mp3', '.wav', '.ogg', '.flac', '.m4a', '.aac', '.opus'];
  if (mimeType.startsWith('audio/') || audioExts.includes(ext)) return 'audio';

  // PDF
  if (mimeType === 'application/pdf' || ext === '.pdf') return 'pdf';

  // Markdown
  if (ext === '.md' || ext === '.markdown' || mimeType.includes('markdown')) return 'markdown';

  // Tabular Data
  if (ext === '.csv' || ext === '.tsv') return 'csv';

  // Presentations
  const pptExts = ['.ppt', '.pptx', '.odp', '.key'];
  if (pptExts.includes(ext) || mimeType.includes('presentation') || mimeType.includes('powerpoint')) {
    return 'presentation';
  }

  // Office Documents & Spreadsheets
  const officeExts = ['.doc', '.docx', '.xls', '.xlsx', '.odt', '.ods'];
  if (
    officeExts.includes(ext) ||
    mimeType.includes('officedocument') ||
    mimeType.includes('msword') ||
    mimeType.includes('excel')
  ) {
    return 'office';
  }

  // Source Code & Configs
  const codeExts = [
    '.js', '.ts', '.jsx', '.tsx', '.py', '.go', '.rs', '.c', '.cpp', '.h', '.hpp',
    '.java', '.kt', '.swift', '.rb', '.php', '.cs', '.scala',
    '.json', '.yaml', '.yml', '.toml', '.xml', '.html', '.htm',
    '.css', '.scss', '.sass', '.less', '.sql', '.sh', '.bash', '.zsh', '.bat', '.ps1',
    '.r', '.dart', '.lua', '.vim'
  ];
  if (
    codeExts.includes(ext) ||
    filename.toLowerCase() === 'dockerfile' ||
    filename.toLowerCase().startsWith('.env')
  ) {
    return 'code';
  }

  // Plain Text / Logs
  const textExts = ['.txt', '.log', '.ini', '.conf', '.cfg', '.nfo'];
  if (textExts.includes(ext) || mimeType.startsWith('text/')) return 'text';

  return 'none';
}

export function formatFileSize(bytes: number): string {
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return (bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0) + ' ' + units[i];
}
