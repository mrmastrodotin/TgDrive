import React, { useState, useMemo } from 'react';
import { Table, Code2, Search } from 'lucide-react';

export interface CsvPreviewProps {
  textContent: string;
  loading: boolean;
}

export const CsvPreview: React.FC<CsvPreviewProps> = ({ textContent, loading }) => {
  const [csvMode, setCsvMode] = useState<'table' | 'raw'>('table');
  const [searchQuery, setSearchQuery] = useState('');

  // Parsed CSV rows and headers
  const parsedCSV = useMemo(() => {
    if (!textContent) return { headers: [], rows: [] };
    const lines = textContent.split(/\r?\n/).filter((l) => l.trim().length > 0);
    if (lines.length === 0) return { headers: [], rows: [] };

    const parseLine = (line: string) => {
      const result: string[] = [];
      let cur = '';
      let inQuotes = false;
      for (let i = 0; i < line.length; i++) {
        const c = line[i];
        if (c === '"') {
          inQuotes = !inQuotes;
        } else if ((c === ',' || c === '\t') && !inQuotes) {
          result.push(cur.trim());
          cur = '';
        } else {
          cur += c;
        }
      }
      result.push(cur.trim());
      return result;
    };

    const headers = parseLine(lines[0]);
    const rows = lines.slice(1).map(parseLine);
    return { headers, rows };
  }, [textContent]);

  const filteredCSVRows = useMemo(() => {
    if (!searchQuery) return parsedCSV.rows;
    const q = searchQuery.toLowerCase();
    return parsedCSV.rows.filter((row) =>
      row.some((cell) => cell.toLowerCase().includes(q))
    );
  }, [parsedCSV.rows, searchQuery]);

  return (
    <div className="preview-csv-viewport">
      <div className="canvas-sub-header">
        <div className="canvas-mode-toggle">
          <button
            className={`mode-tab-btn ${csvMode === 'table' ? 'active' : ''}`}
            onClick={() => setCsvMode('table')}
          >
            <Table size={14} />
            <span>Table View</span>
          </button>
          <button
            className={`mode-tab-btn ${csvMode === 'raw' ? 'active' : ''}`}
            onClick={() => setCsvMode('raw')}
          >
            <Code2 size={14} />
            <span>Raw Text</span>
          </button>
        </div>

        <div className="search-in-code-box">
          <Search size={14} className="search-icon" />
          <input
            type="text"
            placeholder="Filter rows..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="search-input-field"
          />
        </div>

        <div className="canvas-stats">
          <span>{parsedCSV.headers.length} columns</span>
          <span>•</span>
          <span>{filteredCSVRows.length} rows</span>
        </div>
      </div>

      {loading ? (
        <div className="preview-loading-state">
          <span className="spinner-border"></span>
          <p>Decrypting dataset...</p>
        </div>
      ) : csvMode === 'table' ? (
        <div className="csv-table-scroll-container">
          <table className="csv-interactive-table">
            <thead>
              <tr>
                <th className="table-index-col">#</th>
                {parsedCSV.headers.map((h, i) => (
                  <th key={i}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {filteredCSVRows.map((row, rIdx) => (
                <tr key={rIdx}>
                  <td className="table-index-col">{rIdx + 1}</td>
                  {row.map((cell, cIdx) => (
                    <td key={cIdx}>{cell}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="raw-code-canvas">
          <pre className="code-pre-block">
            <code>{textContent}</code>
          </pre>
        </div>
      )}
    </div>
  );
};
