package image

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"quran-images/db"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
)

const (
	FontDefault   = "QCF_BSML.TTF"
	FontFactor    = 21.0
	FontFactor270 = 22.5
)

var PHI = (math.Sqrt(5) + 1) / 2

type PageGenerator struct {
	db      *db.DB
	fontDir string
	output  string
	width   int
}

type BoundingBox struct {
	CoordX   float64
	CoordY   float64
	MinX     float64
	MaxX     float64
	MinY     float64
	MaxY     float64
	Space    float64
	CharDown float64
	CharUp   float64
	Width    float64
	Height   float64
	PtSize   float64
}

type Page struct {
	Number    int
	Width     int
	Height    float64
	PtSize    int
	MarginTop float64
	CoordX    float64
	CoordY    float64
	Font      string
	Image     *gg.Context
	Color     map[string]color.Color
	Lines     []*db.Line
}

type LineContext struct {
	Line  *db.Line
	Page  *Page
	Box   *BoundingBox
	PrevW float64
}

type GlyphContext struct {
	Glyph     *db.Glyph
	Line      *LineContext
	Page      *Page
	Box       *BoundingBox
	UseCoordX bool
	UseCoordY bool
	YOffset   float64
}

func NewPageGenerator(db *db.DB, fontDir, output string, width int) *PageGenerator {
	return &PageGenerator{
		db:      db,
		fontDir: fontDir,
		output:  output,
		width:   width,
	}
}

func (pg *PageGenerator) Generate(pages []int) error {
	// Reset bounding box table
	if err := pg.db.ResetBoundingBoxTable(); err != nil {
		return fmt.Errorf("failed to reset bounding box table: %w", err)
	}

	// Process pages in reverse order (like Perl)
	for i := len(pages) - 1; i >= 0; i-- {
		pageNum := pages[i]
		if pageNum >= 1 && pageNum <= 604 {
			img, err := pg.Create(pageNum)
			if err != nil {
				return fmt.Errorf("failed to create page %d: %w", pageNum, err)
			}

			if err := pg.Write(pageNum, img); err != nil {
				return fmt.Errorf("failed to write page %d: %w", pageNum, err)
			}
		}
	}

	return nil
}

func (pg *PageGenerator) Create(pageNumber int) (*gg.Context, error) {
	fmt.Printf("Page: %d\n", pageNumber)

	fontFactor := FontFactor
	fontDelta := 1.0

	// Page 270 font is slightly larger
	if pageNumber == 270 {
		fontFactor = FontFactor270
	}

	pageHeight := float64(pg.width) * PHI * fontDelta
	page := &Page{
		Number:    pageNumber,
		Width:     pg.width,
		Height:    pageHeight,
		PtSize:    int(float64(pg.width) / fontFactor),
		MarginTop: float64(int(float64(pg.width)/fontFactor)) / 2.0,
		CoordY:    0,
		Font:      filepath.Join(pg.fontDir, FontDefault),
		Color: map[string]color.Color{
			"white": color.RGBA{255, 255, 255, 127},
			"black": color.RGBA{0, 0, 0, 255},
			"red":   color.RGBA{19, 50, 112, 255},
		},
	}

	// In Perl: coord_y starts at margin_top (from top)
	// But stringFT uses bottom-left origin, so we calculate from bottom
	// For gg, we'll keep coordY from top and convert when rendering
	page.CoordY = page.MarginTop

	// Create image context
	// In Perl: white with alpha 127 is set as transparent background
	img := gg.NewContext(page.Width, int(page.Height))
	// Fill with white background (transparent in PNG export)
	img.SetColor(color.RGBA{255, 255, 255, 0})
	img.Clear()
	page.Image = img

	// Get page lines from database
	lines, err := pg.db.GetPageLines(pageNumber, pg.fontDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get page lines: %w", err)
	}
	page.Lines = lines

	// Debug: check if we have lines
	if len(lines) == 0 {
		fmt.Printf("Warning: No lines found for page %d\n", pageNumber)
		return img, nil
	}

	// Debug: count total glyphs
	totalGlyphs := 0
	for _, line := range lines {
		totalGlyphs += len(line.Glyphs)
	}
	fmt.Printf("Found %d lines with %d total glyphs for page %d\n", len(lines), totalGlyphs, pageNumber)

	// Process each line
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		lineCtx := &LineContext{
			Line:  line,
			Page:  page,
			PrevW: 0,
		}

		// Calculate line box first (needed for coord_x calculation)
		lineCtx.Box = pg.getBox(lineCtx, nil)

		// Debug first line
		if i == 0 {
			fmt.Printf("First line: type=%s, glyphs=%d, font=%s, lineBoxWidth=%.2f, lineBoxCoordX=%.2f\n",
				line.Type, len(line.Glyphs), filepath.Base(line.Font), lineCtx.Box.Width, lineCtx.Box.CoordX)
		}

		// Adjust coord_y if needed
		if page.CoordY <= page.MarginTop && lineCtx.Box.MinY < 0 {
			page.CoordY -= lineCtx.Box.MinY
		}

		// Reset coord_x for each line (Perl: $page->{coord_x} = 0)
		page.CoordX = 0.0
		lineCtx.PrevW = 0.0

		// Process each glyph in the line
		for j := 0; j < len(line.Glyphs); j++ {
			glyph := &line.Glyphs[j]
			glyphCtx := &GlyphContext{
				Glyph: glyph,
				Line:  lineCtx,
				Page:  page,
			}

			glyphCtx.Box = pg.getBox(lineCtx, glyphCtx)

			// Debug: calculate total line width from all glyphs
			if j == 0 && i == 0 {
				// Calculate total width of all glyphs in line
				totalGlyphWidth := 0.0
				for k := 0; k < len(line.Glyphs); k++ {
					tempGlyphCtx := &GlyphContext{
						Glyph: &line.Glyphs[k],
						Line:  lineCtx,
						Page:  page,
					}
					tempBox := pg.getBox(lineCtx, tempGlyphCtx)
					totalGlyphWidth += tempBox.MaxX
				}
				fmt.Printf("First line total glyph width: %.2f, line box width: %.2f, page width: %d\n",
					totalGlyphWidth, lineCtx.Box.Width, page.Width)
			}

			// Special handling for pages 1 and 2
			if line.Type != "sura" && (pageNumber == 1 || pageNumber == 2) {
				glyphCtx.UseCoordY = true
				glyphCtx.Box.CoordY += 100
				glyphCtx.YOffset = 100
			}

			// Handle sura header ornament
			if glyph.Position == 1 && line.Type == "sura" {
				ornament, err := pg.db.GetOrnamentGlyph("header-box")
				if err == nil {
					ornamentCtx := &GlyphContext{
						Glyph: &db.Glyph{
							Code:     ornament.Code,
							Text:     ornament.Text,
							Type:     ornament.Type,
							Position: ornament.Position,
						},
						Line: lineCtx,
						Page: page,
					}

					// Initialize Box with ptsize 1.8x (matching Perl: $glyph->{ptsize} = $page->{ptsize} * 1.8)
					ornamentCtx.Box = &BoundingBox{
						PtSize: float64(page.PtSize) * 1.8,
					}

					// Calculate box with the correct ptsize - this will center ornament based on its own width
					// Matching Perl: $glyph->{box} = $self->_get_box($glyph);
					ornamentCtx.Box = pg.getBox(lineCtx, ornamentCtx)

					// Use coord_x and coord_y from ornament's own box (centered based on ornament width)
					// Matching Perl: $glyph->{use_coords} = 1;
					ornamentCtx.UseCoordX = true
					ornamentCtx.UseCoordY = true

					// Calculate vertical center position for ornament
					// Ornament should be vertically centered relative to sura text
					// Sura text baseline: page.CoordY + (lineCtx.Box.Height / 7.0)
					// Sura text center (vertical): baseline + (suraTextHeight / 2) - ascent
					// Ornament center should align with sura text center

					// First, calculate sura text baseline
					suraTextBaseline := page.CoordY + (lineCtx.Box.Height / 7.0)

					// Calculate sura text vertical center
					// Sura text height is approximately lineCtx.Box.Height (CharUp + CharDown)
					suraTextHeight := lineCtx.Box.CharUp + lineCtx.Box.CharDown
					suraTextCenter := suraTextBaseline - (suraTextHeight / 2.0) + lineCtx.Box.CharUp

					// Calculate ornament center (vertical)
					ornamentHeight := ornamentCtx.Box.CharUp + ornamentCtx.Box.CharDown
					ornamentCenter := ornamentCtx.Box.CharUp - (ornamentHeight / 2.0)

					// Set ornament baseline so its center aligns with sura text center
					// ornamentBaseline = suraTextCenter - ornamentCenter
					ornamentCtx.Box.CoordY = suraTextCenter - ornamentCenter
					// Adjust for baseline: subtract charDown to get baseline position (matching Perl)
					ornamentCtx.Box.CoordY -= ornamentCtx.Box.CharDown / 5 * 3

					// Render ornament
					// Matching Perl: $self->_set_box($glyph);
					pg.setBox(ornamentCtx)
				}
			}

			// Calculate coord_x (Perl logic: $page->{coord_x} = $page->{coord_x} ? $page->{coord_x} + $line->{previous_w} : $line->{box}->{coord_x})
			// In Perl: if coord_x is already set (not 0), add previous_w, else use line box coord_x
			// IMPORTANT: For first glyph, use line box coord_x (centered based on entire line width)
			// This ensures the line starts from the correct position
			if page.CoordX != 0.0 {
				// For subsequent glyphs: coord_x = previous coord_x + previous_w
				page.CoordX = page.CoordX + lineCtx.PrevW
			} else {
				// For first glyph in line: use line box coord_x (centered based on entire line width)
				// This matches Perl behavior exactly
				page.CoordX = lineCtx.Box.CoordX
			}

			// Update previous_w for next glyph (Perl: $line->{previous_w} = $glyph->{box}->{max_x})
			// This is the max_x (right edge) of current glyph, used for positioning next glyph
			lineCtx.PrevW = glyphCtx.Box.MaxX

			// Debug first glyph coord_x
			if j == 0 && i == 0 {
				fmt.Printf("First glyph coord_x: page.CoordX=%.2f, glyphCtx.Box.CoordX=%.2f, glyphCtx.Box.Width=%.2f, lineCtx.Box.CoordX=%.2f, page.Width=%d\n",
					page.CoordX, glyphCtx.Box.CoordX, glyphCtx.Box.Width, lineCtx.Box.CoordX, page.Width)
			}

			// Handle sura type
			if line.Type == "sura" {
				glyphCtx.UseCoordY = true
				glyphCtx.Box.CoordY = page.CoordY
				glyphCtx.Box.CoordY += lineCtx.Box.Height / 7.0
			}

			pg.setBox(glyphCtx)

			// Update line box
			lineCtx.Box = pg.getMaxBox(glyphCtx.Box, lineCtx.Box)
		}

		// Update page coord_y
		page.CoordY -= lineCtx.Box.CharDown

		if pageNumber == 1 || pageNumber == 2 {
			page.CoordY += PHI * lineCtx.Box.CharUp
		} else {
			page.CoordY += 2 * lineCtx.Box.CharUp
		}
	}

	return img, nil
}

func (pg *PageGenerator) getBox(lineCtx *LineContext, glyphCtx *GlyphContext) *BoundingBox {
	var fontPath string
	var ptSize float64
	var text string

	if glyphCtx != nil {
		// Glyph box
		if glyphCtx.Glyph.Font != "" {
			fontPath = glyphCtx.Glyph.Font
		} else if lineCtx.Line.Font != "" {
			fontPath = lineCtx.Line.Font
		} else {
			fontPath = lineCtx.Page.Font
		}

		if glyphCtx.Box != nil && glyphCtx.Box.PtSize > 0 {
			ptSize = glyphCtx.Box.PtSize
		} else {
			ptSize = float64(lineCtx.Page.PtSize)
		}

		text = glyphCtx.Glyph.Text
	} else {
		// Line box
		fontPath = lineCtx.Line.Font
		ptSize = float64(lineCtx.Page.PtSize)
		text = lineCtx.Line.Text
	}

	// Load font
	fontData, err := os.ReadFile(fontPath)
	if err != nil {
		// Fallback to default font
		fontPath = lineCtx.Page.Font
		fontData, err = os.ReadFile(fontPath)
		if err != nil {
			return &BoundingBox{}
		}
	}

	ttf, err := truetype.Parse(fontData)
	if err != nil {
		return &BoundingBox{}
	}

	// Handle font adjustment for specific fonts
	shouldAdjust := false
	var adjustedUp, adjustedDown float64

	fontName := filepath.Base(fontPath)
	if strings.Contains(fontName, "P105") || strings.Contains(fontName, "P237") ||
		strings.Contains(fontName, "P552") || strings.Contains(fontName, "P554") {
		shouldAdjust = true
		// Use default font for adjustment values
		defaultFontData, err := os.ReadFile(lineCtx.Page.Font)
		if err == nil {
			defaultTTF, err := truetype.Parse(defaultFontData)
			if err == nil {
				face := truetype.NewFace(defaultTTF, &truetype.Options{Size: ptSize})
				metrics := face.Metrics()
				adjustedUp = float64(metrics.Ascent) / 64.0
				adjustedDown = float64(metrics.Descent) / 64.0
			}
		}
	}

	// Create face for measurements
	face := truetype.NewFace(ttf, &truetype.Options{Size: ptSize})
	metrics := face.Metrics()

	charUp := float64(metrics.Ascent) / 64.0
	charDown := float64(metrics.Descent) / 64.0
	space := float64(metrics.Height) / 64.0

	if shouldAdjust {
		charUp = adjustedUp
		charDown = adjustedDown
	}

	// Calculate bounding box
	// Convert HTML entity to actual character
	actualText := pg.htmlEntityToRune(text)

	// Get advance width (total width of text)
	// For line box, actualText contains all glyphs concatenated (multiple HTML entities)
	// We need to measure the actual rendered text
	advance := font.MeasureString(face, actualText)
	width := float64(advance) / 64.0

	// Get bounding box for the glyph using freetype
	// Similar to GD::Image->stringFT which returns 8 values representing corners
	// GD::Image->stringFT($color, $font, $ptsize, 0, 0, 0, $text) returns:
	// [0,1] Lower left corner (x,y)
	// [2,3] Lower right corner (x,y)
	// [4,5] Upper right corner (x,y)
	// [6,7] Upper left corner (x,y)
	// Perl: $min_x = min($bbox[0], $bbox[6]); $max_x = max($bbox[4], $bbox[2]);
	//       $width = $max_x; (right edge position, not actual width)
	var minX, maxX, minY, maxY float64

	if len(actualText) > 0 {
		// For accurate bounds, we need to measure the actual rendered text
		// In Perl, max_x is the maximum of bbox[4] and bbox[2] (upper right and lower right X)
		// This represents the right edge position of the rendered text
		// For now, we use advance width as maxX (right edge position)
		// This is accurate for most cases where text doesn't have negative bearings
		maxX = width

		// min_x is the minimum of bbox[0] and bbox[6] (lower left and upper left X)
		// This is usually 0 or negative for most fonts
		// For Arabic fonts, minX is typically 0 or slightly negative
		minX = 0.0

		// Y bounds from font metrics
		// min_y = min($bbox[7], $bbox[5]); max_y = max($bbox[1], $bbox[3]);
		minY = -charUp
		maxY = charDown
	} else {
		minX = 0.0
		maxX = width
		minY = -charUp
		maxY = charDown
	}

	// Debug: log width calculation for first line
	if glyphCtx == nil && len(lineCtx.Line.Glyphs) > 0 {
		// This is line box calculation
		originalTextLen := len(text)
		parsedTextLen := len(actualText)
		fmt.Printf("Line box width calculation: original text length=%d, parsed text length=%d chars, advance=%d, width=%.2f, maxX=%.2f\n",
			originalTextLen, parsedTextLen, advance, width, maxX)

		// Warn if width is unreasonably large
		if width > float64(lineCtx.Page.Width)*2 {
			fmt.Printf("WARNING: Line box width (%.2f) is much larger than page width (%d)!\n",
				width, lineCtx.Page.Width)
			fmt.Printf("  Original text: %s\n", truncateString(text, 100))
			fmt.Printf("  Parsed text: %s\n", truncateString(actualText, 100))
		}
	}

	// Estimate height
	height := charUp + charDown

	// Calculate coord_x: center the text horizontally
	// Perl: $width = $max_x; (not $max_x - $min_x)
	//       $coord_x = ($page->{width} - $width) / 2;
	// So width is the right edge position (max_x), not the actual width
	// This means coord_x centers based on where the right edge should be
	// For rendering, we need to adjust: actual coord_x = coord_x - min_x (to account for left offset)

	// Use maxX as width (right edge position), matching Perl behavior exactly
	// Perl: $width = $max_x; $coord_x = ($page->{width} - $width) / 2;
	// So coord_x centers the right edge of the text
	widthForCoord := maxX
	if widthForCoord == 0 {
		widthForCoord = width
	}

	// Calculate coord_x to center the text
	// This centers based on the right edge (maxX)
	coordX := (float64(lineCtx.Page.Width) - widthForCoord) / 2.0
	coordY := lineCtx.Page.CoordY

	// Debug: log coord_x calculation for line box
	if glyphCtx == nil {
		fmt.Printf("Line box coord_x: pageWidth=%d, maxX=%.2f, width=%.2f, widthForCoord=%.2f, coordX=%.2f, minX=%.2f\n",
			lineCtx.Page.Width, maxX, width, widthForCoord, coordX, minX)

		// Warn if coordX is negative (text will be off-screen)
		if coordX < 0 {
			fmt.Printf("WARNING: coordX is negative (%.2f), text will be cut off on the right!\n", coordX)
			fmt.Printf("  This means line width (%.2f) is larger than page width (%d)\n",
				widthForCoord, lineCtx.Page.Width)
		}
	}

	return &BoundingBox{
		CoordX:   coordX,
		CoordY:   coordY,
		MinX:     minX,
		MaxX:     maxX,
		MinY:     minY,
		MaxY:     maxY,
		Space:    space,
		CharDown: charDown,
		CharUp:   charUp,
		Width:    width,
		Height:   height,
		PtSize:   ptSize,
	}
}

func (pg *PageGenerator) setBox(glyphCtx *GlyphContext) {
	lineCtx := glyphCtx.Line
	page := glyphCtx.Page

	fontPath := lineCtx.Line.Font
	if glyphCtx.Glyph.Font != "" {
		fontPath = glyphCtx.Glyph.Font
	}

	ptSize := float64(page.PtSize)
	if glyphCtx.Box.PtSize > 0 {
		ptSize = glyphCtx.Box.PtSize
	}

	// Load font
	fontData, err := os.ReadFile(fontPath)
	if err != nil {
		// Fallback to default font
		fontPath = page.Font
		fontData, err = os.ReadFile(fontPath)
		if err != nil {
			fmt.Printf("Warning: Failed to load font %s: %v\n", fontPath, err)
			return
		}
	}

	ttf, err := truetype.Parse(fontData)
	if err != nil {
		fmt.Printf("Warning: Failed to parse font %s: %v\n", fontPath, err)
		return
	}

	// Debug: log font loading success for first glyph
	if glyphCtx.Glyph.PageLineID == page.Lines[0].Glyphs[0].PageLineID {
		fmt.Printf("Font loaded successfully: %s, size: %d bytes\n", filepath.Base(fontPath), len(fontData))
	}

	// Determine color
	textColor := page.Color["black"]
	if lineCtx.Line.Type == "ayah" {
		// Color logic can be added here if needed
		textColor = page.Color["black"]
	} else if lineCtx.Line.Type == "sura" {
		// textColor = page.Color["red"] // Commented out in Perl
		textColor = page.Color["black"]
	}

	// Calculate coordinates (Perl logic in _set_box)
	// Perl: ($coord_x, $coord_y) = $glyph->{use_coords} ? ($glyph->{box}->{coord_x}, $glyph->{box}->{coord_y}) : ($page->{coord_x}, $page->{coord_y});
	//       $coord_x = $glyph->{box}->{coord_x} if $glyph->{use_coord_x};
	//       $coord_y = $glyph->{box}->{coord_y} if $glyph->{use_coord_y};
	coordX := page.CoordX
	coordY := page.CoordY

	if glyphCtx.UseCoordX {
		coordX = glyphCtx.Box.CoordX
	}
	if glyphCtx.UseCoordY {
		coordY = glyphCtx.Box.CoordY
	}

	// Debug: log coord_x calculation for first few glyphs
	if glyphCtx.Glyph.PageLineID == page.Lines[0].Glyphs[0].PageLineID {
		fmt.Printf("CoordX calculation: page.CoordX=%.2f, glyphCtx.Box.CoordX=%.2f, UseCoordX=%v, final coordX=%.2f\n",
			page.CoordX, glyphCtx.Box.CoordX, glyphCtx.UseCoordX, coordX)
	}

	// Calculate bounding box for database
	if lineCtx.Line.Type == "ayah" {
		minX := page.CoordX + glyphCtx.Box.MinX
		minXInt := int(minX)
		maxX := minXInt + int(glyphCtx.Box.MaxX-glyphCtx.Box.MinX)
		//maxXInt := int(maxX + 0.5) // Convert to int and round
		maxXInt := int(maxX) // Convert to int and round

		minY := page.CoordY + glyphCtx.Box.MinY
		minYInt := int(minY)
		maxY := minYInt + int(glyphCtx.Box.MaxY-glyphCtx.Box.MinY)
		//maxYInt := int(maxY + 0.5)
		maxYInt := int(maxY) // Convert to int and round

		if glyphCtx.YOffset != 0 {
			minYInt += int(glyphCtx.YOffset)
			maxYInt += int(glyphCtx.YOffset)
		}

		// Save to database
		pg.db.SetPageLineBbox(glyphCtx.Glyph.PageLineID, page.Width, minXInt, maxXInt, minYInt, maxYInt)
	}

	// Render text using gg (which uses freetype internally)
	actualText := pg.htmlEntityToRune(glyphCtx.Glyph.Text)

	// Skip empty text
	if actualText == "" || actualText == "&#0;" || len(actualText) == 0 {
		if glyphCtx.Glyph.PageLineID == page.Lines[0].Glyphs[0].PageLineID {
			fmt.Printf("Warning: Empty text for first glyph, original: %s\n", glyphCtx.Glyph.Text)
		}
		return
	}

	// Debug: check if text is valid
	if len([]rune(actualText)) == 0 {
		if glyphCtx.Glyph.PageLineID == page.Lines[0].Glyphs[0].PageLineID {
			fmt.Printf("Warning: Text converted to empty rune, original: %s\n", glyphCtx.Glyph.Text)
		}
		return
	}

	// Set color and font face
	page.Image.SetColor(textColor)
	face := truetype.NewFace(ttf, &truetype.Options{Size: ptSize})
	page.Image.SetFontFace(face)

	// Draw text
	// In Perl/GD: stringFT uses bottom-left origin (Y=0 at bottom, Y increases upward)
	// Perl calculates coord_y from top (starting at margin_top), but passes it directly to stringFT
	// So stringFT interprets coord_y as Y from bottom of image
	//
	// For gg: DrawStringAnchored uses top-left origin (Y=0 at top, Y increases downward)
	// Anchor (0, 1) means the anchor point is at left-bottom of the text
	// So if we want text at coordY from top, we need to pass coordY directly
	// BUT: we need to account for the baseline
	imgHeight := float64(page.Image.Height())

	// Get font metrics to adjust for baseline
	metrics := face.Metrics()
	ascent := float64(metrics.Ascent) / 64.0

	// For anchor (0, 1), the Y coordinate represents the bottom of the text
	// coordY is the baseline position from top, so we need to add ascent to get bottom
	// Actually, let's try a simpler approach: use coordY directly with anchor (0, 0) and adjust
	// Or use DrawString with manual baseline calculation

	// Try: use coordY directly with anchor (0, 0) which is top-left
	// But we need baseline, so add ascent
	baselineY := coordY + ascent

	// Debug first glyph
	isFirstGlyph := len(page.Lines) > 0 && len(page.Lines[0].Glyphs) > 0 &&
		glyphCtx.Glyph.PageLineID == page.Lines[0].Glyphs[0].PageLineID

	if isFirstGlyph {
		fmt.Printf("First glyph: text=%q, coordX=%.2f, coordY=%.2f, baselineY=%.2f, ascent=%.2f, imgHeight=%.2f, ptSize=%.2f\n",
			actualText, coordX, coordY, baselineY, ascent, imgHeight, ptSize)
	}

	// Ensure coordinates are within reasonable bounds
	if coordX < -100 || coordX > float64(page.Width)+100 || baselineY < -100 || baselineY > imgHeight+100 {
		if isFirstGlyph {
			fmt.Printf("Warning: First glyph out of bounds, skipping. coordX=%.2f, baselineY=%.2f\n", coordX, baselineY)
		}
		return
	}

	// Render text
	// In Perl/GD: stringFT renders at coord_x, coord_y directly
	// coord_x is calculated to center the right edge (maxX)
	// stringFT renders from baseline at coord_x, which means the text starts at coord_x + minX
	// For gg: coordX centers based on maxX, we need to render at coordX + minX
	// But actually, in Perl, coord_x is the baseline position, and stringFT renders from there
	// So we should use coordX directly, but account for the fact that gg renders from left
	// The actual render position should be: coordX (which is the baseline X for the right edge)
	// But we need to subtract the width to get the left edge, or add minX to get the actual start
	// Actually, coordX in Perl is the X position for rendering, so we use it directly
	// EXCEPTION: For ornament with use_coord_x, coordX is already the correct centered position
	// So we should use coordX directly without adding minX
	renderX := coordX
	if !glyphCtx.UseCoordX {
		// For regular glyphs, adjust by minX
		renderX = coordX + glyphCtx.Box.MinX
	}

	// Use left-top anchor (0, 0) for baseline position
	page.Image.DrawStringAnchored(actualText, renderX, baselineY, 0, 0)

	// Debug: verify rendering
	if isFirstGlyph {
		width, height := page.Image.MeasureString(actualText)
		fmt.Printf("Text rendered: %q, measured width=%.2f, height=%.2f\n", actualText, width, height)
	}
}

func (pg *PageGenerator) getMaxBox(boxA, boxB *BoundingBox) *BoundingBox {
	result := &BoundingBox{}

	// Min values
	if boxA.MinX <= boxB.MinX {
		result.MinX = boxA.MinX
	} else {
		result.MinX = boxB.MinX
	}

	if boxA.MinY <= boxB.MinY {
		result.MinY = boxA.MinY
	} else {
		result.MinY = boxB.MinY
	}

	if boxA.CharDown <= boxB.CharDown {
		result.CharDown = boxA.CharDown
	} else {
		result.CharDown = boxB.CharDown
	}

	if boxA.CoordX <= boxB.CoordX {
		result.CoordX = boxA.CoordX
	} else {
		result.CoordX = boxB.CoordX
	}

	if boxA.CoordY <= boxB.CoordY {
		result.CoordY = boxA.CoordY
	} else {
		result.CoordY = boxB.CoordY
	}

	// Max values
	if boxA.MaxX >= boxB.MaxX {
		result.MaxX = boxA.MaxX
	} else {
		result.MaxX = boxB.MaxX
	}

	if boxA.MaxY >= boxB.MaxY {
		result.MaxY = boxA.MaxY
	} else {
		result.MaxY = boxB.MaxY
	}

	if boxA.Space >= boxB.Space {
		result.Space = boxA.Space
	} else {
		result.Space = boxB.Space
	}

	if boxA.CharUp >= boxB.CharUp {
		result.CharUp = boxA.CharUp
	} else {
		result.CharUp = boxB.CharUp
	}

	if boxA.Width >= boxB.Width {
		result.Width = boxA.Width
	} else {
		result.Width = boxB.Width
	}

	if boxA.Height >= boxB.Height {
		result.Height = boxA.Height
	} else {
		result.Height = boxB.Height
	}

	return result
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func (pg *PageGenerator) htmlEntityToRune(htmlEntity string) string {
	// Convert HTML entities to actual Unicode characters
	// Handle both single entity (&#1234;) and multiple entities (&#64399;&#64396;)
	result := ""
	remaining := htmlEntity

	for {
		// Find the start of an entity
		startIdx := strings.Index(remaining, "&#")
		if startIdx == -1 {
			// No more entities, append remaining text
			result += remaining
			break
		}

		// Append text before the entity
		result += remaining[:startIdx]
		remaining = remaining[startIdx:]

		// Find the end of the entity (semicolon)
		endIdx := strings.Index(remaining, ";")
		if endIdx == -1 {
			// No semicolon found, treat as regular text
			result += remaining
			break
		}

		// Extract entity: &#1234;
		entity := remaining[:endIdx+1]
		remaining = remaining[endIdx+1:]

		// Parse entity code
		codeStr := strings.TrimPrefix(entity, "&#")
		codeStr = strings.TrimSuffix(codeStr, ";")
		code, err := strconv.Atoi(codeStr)
		if err == nil {
			result += string(rune(code))
		} else {
			// If parsing fails, keep the original entity (shouldn't happen normally)
			result += entity
		}
	}

	return result
}

func (pg *PageGenerator) Write(pageNumber int, img *gg.Context) error {
	// Create output directory
	if err := os.MkdirAll(pg.output, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write PNG file
	filename := filepath.Join(pg.output, fmt.Sprintf("%d.png", pageNumber))
	return img.SavePNG(filename)
}
