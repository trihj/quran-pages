package db

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
	"quran-images/config"
)

type DB struct {
	conn *sql.DB
}

type PageLine struct {
	GlyphPageLineID int
	LineNumber      int
	LineType        string
	Position        int
	FontFile        string
	GlyphCode       int
	GlyphType       sql.NullString
}

type Line struct {
	Number int
	Type   string
	Text   string
	Font   string
	Glyphs []Glyph
}

type Glyph struct {
	PageLineID int
	Code       int
	Text       string
	Type       string
	Position   int
	Font       string
}

type OrnamentGlyph struct {
	Code     int
	Text     string
	Type     string
	Position int
}

func NewDB(cfg *config.Config) (*DB, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.Username,
		cfg.Database.Password,
		cfg.Database.Database,
	)

	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{conn: conn}, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

func (d *DB) ResetBoundingBoxTable() error {
	_, err := d.conn.Exec("TRUNCATE glyph_page_line_bbox")
	return err
}

func (d *DB) GetPageLines(pageNumber int, fontDir string) ([]*Line, error) {
	query := `
		SELECT gpl.glyph_page_line_id, gpl.line_number, gpl.line_type, gpl.position, g.font_file,
			g.glyph_code, gt.name
		FROM glyph_page_line gpl
		LEFT JOIN glyph g ON g.glyph_id = gpl.glyph_id
		LEFT JOIN glyph_type gt ON g.glyph_type_id = gt.glyph_type_id
		WHERE gpl.page_number = $1
		ORDER BY gpl.page_number ASC, gpl.line_number ASC, gpl.position DESC
	`

	rows, err := d.conn.Query(query, pageNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to query page lines: %w", err)
	}
	defer rows.Close()

	linesMap := make(map[int]*Line)

	for rows.Next() {
		var pl PageLine
		var glyphType sql.NullString

		err := rows.Scan(
			&pl.GlyphPageLineID,
			&pl.LineNumber,
			&pl.LineType,
			&pl.Position,
			&pl.FontFile,
			&pl.GlyphCode,
			&glyphType,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		pl.GlyphType = glyphType

		glyphText := fmt.Sprintf("&#%d;", pl.GlyphCode)
		glyphTypeStr := ""
		if glyphType.Valid {
			glyphTypeStr = glyphType.String
		}

		lineNum := pl.LineNumber - 1 // Convert to 0-based index
		glyphFont := ""
		if pl.FontFile != "" {
			glyphFont = fmt.Sprintf("%s/%s", fontDir, pl.FontFile)
		}
		
		if linesMap[lineNum] == nil {
			linesMap[lineNum] = &Line{
				Number: pl.LineNumber,
				Type:   pl.LineType,
				Text:   glyphText,
				Font:   fmt.Sprintf("%s/%s", fontDir, pl.FontFile),
				Glyphs: []Glyph{
					{
						PageLineID: pl.GlyphPageLineID,
						Code:       pl.GlyphCode,
						Text:       glyphText,
						Type:       glyphTypeStr,
						Position:   pl.Position,
						Font:       glyphFont,
					},
				},
			}
		} else {
			linesMap[lineNum].Text += glyphText
			linesMap[lineNum].Glyphs = append(linesMap[lineNum].Glyphs, Glyph{
				PageLineID: pl.GlyphPageLineID,
				Code:       pl.GlyphCode,
				Text:       glyphText,
				Type:       glyphTypeStr,
				Position:   pl.Position,
				Font:       glyphFont,
			})
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	// Convert map to slice, maintaining order
	lines := make([]*Line, len(linesMap))
	for i := 0; i < len(linesMap); i++ {
		lines[i] = linesMap[i]
	}

	return lines, nil
}

func (d *DB) GetOrnamentGlyph(name string) (*OrnamentGlyph, error) {
	query := `
		SELECT g.glyph_code, gt.name
		FROM glyph g, glyph_type gt, glyph_type gtp
		WHERE g.glyph_type_id = gt.glyph_type_id
		AND gt.parent_id = gtp.glyph_type_id
		AND gtp.name = 'ornament'
		AND gt.name = $1
		LIMIT 1
	`

	var glyphCode int
	var glyphType string

	err := d.conn.QueryRow(query, name).Scan(&glyphCode, &glyphType)
	if err != nil {
		return nil, fmt.Errorf("failed to get ornament glyph: %w", err)
	}

	return &OrnamentGlyph{
		Code:     glyphCode,
		Text:     fmt.Sprintf("&#%d;", glyphCode),
		Type:     glyphType,
		Position: 0,
	}, nil
}

func (d *DB) SetPageLineBbox(glyphPageLineID, imgWidth, minX, maxX, minY, maxY int) error {
	// Check if record exists
	var bboxID sql.NullInt64
	checkQuery := "SELECT glyph_page_bbox_id FROM glyph_page_line_bbox WHERE glyph_page_line_id = $1 AND img_width = $2"
	err := d.conn.QueryRow(checkQuery, glyphPageLineID, imgWidth).Scan(&bboxID)

	if err == sql.ErrNoRows {
		// Insert new record
		insertQuery := `
			INSERT INTO glyph_page_line_bbox (glyph_page_line_id, img_width, min_x, max_x, min_y, max_y)
			VALUES ($1, $2, $3, $4, $5, $6)
		`
		_, err = d.conn.Exec(insertQuery, glyphPageLineID, imgWidth, minX, maxX, minY, maxY)
		return err
	} else if err != nil {
		return fmt.Errorf("failed to check bbox: %w", err)
	}

	// Update existing record
	updateQuery := `
		UPDATE glyph_page_line_bbox
		SET min_x = $1, max_x = $2, min_y = $3, max_y = $4
		WHERE glyph_page_line_id = $5 AND img_width = $6
	`
	_, err = d.conn.Exec(updateQuery, minX, maxX, minY, maxY, glyphPageLineID, imgWidth)
	return err
}
