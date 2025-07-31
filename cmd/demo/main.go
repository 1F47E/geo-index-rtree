package main

import (
	"fmt"
	"log"
	"math/rand"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kass/go-geo-index/pkg/models"
	"github.com/kass/go-geo-index/pkg/rtree"
)

const (
	indexFile = "geo_index.gob"
)

var (
	// Styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF79C6")).
			Background(lipgloss.Color("#282A36")).
			Padding(0, 1).
			MarginTop(1).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#8BE9FD"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#50FA7B"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5555"))

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F1FA8C"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6272A4"))

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#BD93F9")).
			Padding(1, 2).
			MarginTop(1).
			MarginBottom(1)

	statStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFB86C"))
)

type stage int

const (
	stageLoading stage = iota
	stageLoadComplete
	stageBenchmarking
	stageBenchmarkComplete
	stageRadiusSearch
	stageRadiusComplete
	stageNearestNeighbor
	stageNearestComplete
	stageDone
)

type model struct {
	stage           stage
	spinner         spinner.Model
	progress        progress.Model
	progressPercent float64
	
	// Loading stats
	pointsLoaded    int
	loadTime        time.Duration
	
	// Benchmark stats
	benchmarkStats  benchmarkResult
	radiusStats     benchmarkResult
	nearestStats    benchmarkResult
	
	// Messages
	messages        []string
	width           int
	height          int
}

type benchmarkResult struct {
	totalQueries    int64
	totalTime       time.Duration
	totalResults    int64
	avgQueryTime    time.Duration
	queriesPerSec   float64
}

type progressMsg float64
type stageCompleteMsg struct {
	stage stage
	stats interface{}
}
type messageMsg string

func initialModel() model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF79C6"))
	
	p := progress.New(progress.WithDefaultGradient())
	
	return model{
		stage:    stageLoading,
		spinner:  s,
		progress: p,
		messages: []string{},
		width:    80,
		height:   24,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		runDemo(),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = msg.Width - 10
		return m, nil
		
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
		
	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd
		
	case progressMsg:
		m.progressPercent = float64(msg)
		return m, m.progress.SetPercent(float64(msg))
		
	case messageMsg:
		m.messages = append(m.messages, string(msg))
		if len(m.messages) > 5 {
			m.messages = m.messages[1:]
		}
		return m, nil
		
	case stageCompleteMsg:
		switch msg.stage {
		case stageLoading:
			if stats, ok := msg.stats.(loadStats); ok {
				m.pointsLoaded = stats.points
				m.loadTime = stats.duration
			}
			m.stage = stageLoadComplete
		case stageBenchmarking:
			if stats, ok := msg.stats.(benchmarkResult); ok {
				m.benchmarkStats = stats
			}
			m.stage = stageBenchmarkComplete
		case stageRadiusSearch:
			if stats, ok := msg.stats.(benchmarkResult); ok {
				m.radiusStats = stats
			}
			m.stage = stageRadiusComplete
		case stageNearestNeighbor:
			if stats, ok := msg.stats.(benchmarkResult); ok {
				m.nearestStats = stats
			}
			m.stage = stageNearestComplete
		}
		
		// Auto-advance to next stage
		if m.stage < stageDone {
			return m, tea.Tick(time.Second, func(t time.Time) tea.Msg {
				m.stage++
				return nil
			})
		}
		return m, nil
	}
	
	return m, nil
}

func (m model) View() string {
	var b strings.Builder
	
	b.WriteString(titleStyle.Render("🌍 Go Geo-Index Demo"))
	b.WriteString("\n\n")
	
	switch m.stage {
	case stageLoading:
		b.WriteString(subtitleStyle.Render("Loading Points"))
		b.WriteString("\n\n")
		b.WriteString(m.spinner.View() + " Loading 1,000,000 random points...\n\n")
		b.WriteString(m.progress.ViewAs(m.progressPercent))
		
	case stageLoadComplete:
		b.WriteString(renderLoadStats(m.pointsLoaded, m.loadTime))
		
	case stageBenchmarking:
		b.WriteString(subtitleStyle.Render("Running Bounding Box Queries"))
		b.WriteString("\n\n")
		b.WriteString(m.spinner.View() + " Executing 1,000 bounding box queries...\n\n")
		b.WriteString(m.progress.ViewAs(m.progressPercent))
		
	case stageBenchmarkComplete:
		b.WriteString(renderBenchmarkStats("Bounding Box Queries", m.benchmarkStats))
		
	case stageRadiusSearch:
		b.WriteString(subtitleStyle.Render("Running Radius Searches"))
		b.WriteString("\n\n")
		b.WriteString(m.spinner.View() + " Executing 1,000 radius searches (50km)...\n\n")
		b.WriteString(m.progress.ViewAs(m.progressPercent))
		
	case stageRadiusComplete:
		b.WriteString(renderBenchmarkStats("Radius Searches", m.radiusStats))
		
	case stageNearestNeighbor:
		b.WriteString(subtitleStyle.Render("Running Nearest Neighbor Searches"))
		b.WriteString("\n\n")
		b.WriteString(m.spinner.View() + " Finding 10 nearest neighbors for 1,000 queries...\n\n")
		b.WriteString(m.progress.ViewAs(m.progressPercent))
		
	case stageNearestComplete:
		b.WriteString(renderBenchmarkStats("Nearest Neighbor Searches", m.nearestStats))
		
	case stageDone:
		b.WriteString(renderSummary(m))
	}
	
	// Show recent messages
	if len(m.messages) > 0 {
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Recent activity:"))
		b.WriteString("\n")
		for _, msg := range m.messages {
			b.WriteString(dimStyle.Render("• " + msg))
			b.WriteString("\n")
		}
	}
	
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render("Press 'q' to quit"))
	
	return b.String()
}

func renderLoadStats(points int, duration time.Duration) string {
	stats := fmt.Sprintf(
		"✓ Loaded %s points in %s\n"+
		"✓ Points per second: %s\n"+
		"✓ Index saved to %s",
		statStyle.Render(fmt.Sprintf("%d", points)),
		statStyle.Render(duration.String()),
		statStyle.Render(fmt.Sprintf("%.0f", float64(points)/duration.Seconds())),
		statStyle.Render(indexFile),
	)
	
	return boxStyle.Render(successStyle.Render("Loading Complete!\n\n") + stats)
}

func renderBenchmarkStats(title string, stats benchmarkResult) string {
	content := fmt.Sprintf(
		"✓ Total queries: %s\n"+
		"✓ Total time: %s\n"+
		"✓ Queries per second: %s\n"+
		"✓ Average query time: %s\n"+
		"✓ Total results found: %s\n"+
		"✓ Average results per query: %s",
		statStyle.Render(fmt.Sprintf("%d", stats.totalQueries)),
		statStyle.Render(stats.totalTime.String()),
		statStyle.Render(fmt.Sprintf("%.0f", stats.queriesPerSec)),
		statStyle.Render(stats.avgQueryTime.String()),
		statStyle.Render(fmt.Sprintf("%d", stats.totalResults)),
		statStyle.Render(fmt.Sprintf("%.1f", float64(stats.totalResults)/float64(stats.totalQueries))),
	)
	
	return boxStyle.Render(successStyle.Render(title+" Complete!\n\n") + content)
}

func renderSummary(m model) string {
	summary := titleStyle.Render("🎉 Demo Complete!")
	summary += "\n\n"
	
	summary += infoStyle.Render("The R-Tree index demonstrated:")
	summary += "\n\n"
	
	features := []string{
		fmt.Sprintf("• Parallel loading using %d CPU cores", runtime.NumCPU()),
		fmt.Sprintf("• Efficient bounding box queries (%s queries/sec)", statStyle.Render(fmt.Sprintf("%.0f", m.benchmarkStats.queriesPerSec))),
		fmt.Sprintf("• Fast radius searches (%s queries/sec)", statStyle.Render(fmt.Sprintf("%.0f", m.radiusStats.queriesPerSec))),
		fmt.Sprintf("• Quick nearest neighbor lookups (%s queries/sec)", statStyle.Render(fmt.Sprintf("%.0f", m.nearestStats.queriesPerSec))),
	}
	
	for _, feature := range features {
		summary += successStyle.Render(feature) + "\n"
	}
	
	summary += "\n"
	summary += boxStyle.Render(
		infoStyle.Render("Performance Summary:\n\n") +
		fmt.Sprintf("Total points indexed: %s\n", statStyle.Render(fmt.Sprintf("%d", m.pointsLoaded))) +
		fmt.Sprintf("Index creation time: %s\n", statStyle.Render(m.loadTime.String())) +
		fmt.Sprintf("Average query performance: %s", statStyle.Render(fmt.Sprintf("~%.0f queries/sec", 
			(m.benchmarkStats.queriesPerSec + m.radiusStats.queriesPerSec + m.nearestStats.queriesPerSec) / 3))),
	)
	
	return summary
}

type loadStats struct {
	points   int
	duration time.Duration
}

func runDemo() tea.Cmd {
	return func() tea.Msg {
		// Run the actual demo in the background
		go executeDemo()
		return nil
	}
}

var program *tea.Program

func executeDemo() {
	// Load phase
	loadAndIndex()
	
	// Benchmark phase
	time.Sleep(500 * time.Millisecond)
	runBenchmarks()
	
	// Radius search phase
	time.Sleep(500 * time.Millisecond)
	runRadiusSearches()
	
	// Nearest neighbor phase
	time.Sleep(500 * time.Millisecond)
	runNearestNeighbors()
}

func loadAndIndex() {
	numPoints := 1000000
	numWorkers := runtime.NumCPU()
	
	// Use global program for sending updates
	
	// Generate points
	points := generateRandomPoints(numPoints)
	
	// Create index
	index := rtree.NewGeoIndex()
	
	start := time.Now()
	
	// Load points with progress
	batchSize := numPoints / numWorkers
	if batchSize < 1 {
		batchSize = 1
	}
	
	var wg sync.WaitGroup
	var loaded atomic.Int32
	
	// Progress updater
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		
		for range ticker.C {
			progress := float64(loaded.Load()) / float64(numPoints)
			program.Send(progressMsg(progress))
			
			if loaded.Load() >= int32(numPoints) {
				break
			}
		}
	}()
	
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		startIdx := i * batchSize
		endIdx := startIdx + batchSize
		if i == numWorkers-1 {
			endIdx = numPoints
		}
		
		go func(batch []*models.Point) {
			defer wg.Done()
			err := index.IndexPoints(batch)
			if err != nil {
				program.Send(messageMsg(fmt.Sprintf("Error indexing batch: %v", err)))
			}
			loaded.Add(int32(len(batch)))
		}(points[startIdx:endIdx])
	}
	
	wg.Wait()
	loadTime := time.Since(start)
	
	// Save index
	if err := index.SaveToFile(indexFile); err != nil {
		program.Send(messageMsg(fmt.Sprintf("Error saving index: %v", err)))
	}
	
	program.Send(stageCompleteMsg{
		stage: stageLoading,
		stats: loadStats{
			points:   numPoints,
			duration: loadTime,
		},
	})
}

func runBenchmarks() {
	
	// Load index
	index := rtree.NewGeoIndex()
	if err := index.LoadFromFile(indexFile); err != nil {
		program.Send(messageMsg(fmt.Sprintf("Error loading index: %v", err)))
		return
	}
	
	numQueries := 1000
	numWorkers := runtime.NumCPU()
	
	// Prepare queries
	queries := make([]struct{ latBL, lonBL, latTR, lonTR float64 }, numQueries)
	for i := 0; i < numQueries; i++ {
		centerLat := rand.Float64()*180 - 90
		centerLon := rand.Float64()*360 - 180
		boxSize := rand.Float64()*1.9 + 0.1
		
		queries[i] = struct{ latBL, lonBL, latTR, lonTR float64 }{
			latBL: centerLat - boxSize/2,
			lonBL: centerLon - boxSize/2,
			latTR: centerLat + boxSize/2,
			lonTR: centerLon + boxSize/2,
		}
	}
	
	// Run benchmark with progress
	var totalResults atomic.Int64
	var queryCount atomic.Int32
	
	start := time.Now()
	
	// Progress updater
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		
		for range ticker.C {
			progress := float64(queryCount.Load()) / float64(numQueries)
			program.Send(progressMsg(progress))
			
			if queryCount.Load() >= int32(numQueries) {
				break
			}
		}
	}()
	
	var wg sync.WaitGroup
	queriesPerWorker := numQueries / numWorkers
	
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		startIdx := w * queriesPerWorker
		endIdx := startIdx + queriesPerWorker
		if w == numWorkers-1 {
			endIdx = numQueries
		}
		
		go func(start, end int) {
			defer wg.Done()
			
			localResults := 0
			for i := start; i < end; i++ {
				q := queries[i]
				box := models.BoundingBox{
					BottomLeft: models.Location{Lat: q.latBL, Lon: q.lonBL},
					TopRight: models.Location{Lat: q.latTR, Lon: q.lonTR},
				}
				results, err := index.QueryBox(box)
				if err == nil {
					localResults += len(results)
				}
				queryCount.Add(1)
			}
			totalResults.Add(int64(localResults))
		}(startIdx, endIdx)
	}
	
	wg.Wait()
	elapsed := time.Since(start)
	
	completedQueries := queryCount.Load()
	program.Send(stageCompleteMsg{
		stage: stageBenchmarking,
		stats: benchmarkResult{
			totalQueries:  int64(completedQueries),
			totalTime:     elapsed,
			totalResults:  totalResults.Load(),
			avgQueryTime:  elapsed / time.Duration(completedQueries),
			queriesPerSec: float64(completedQueries) / elapsed.Seconds(),
		},
	})
}

func runRadiusSearches() {
	
	// Load index
	index := rtree.NewGeoIndex()
	if err := index.LoadFromFile(indexFile); err != nil {
		program.Send(messageMsg(fmt.Sprintf("Error loading index: %v", err)))
		return
	}
	
	numQueries := 1000
	numWorkers := runtime.NumCPU()
	searchRadius := 50.0 // km
	
	// Prepare center points
	centers := make([]struct{ lat, lon float64 }, numQueries)
	for i := 0; i < numQueries; i++ {
		centers[i] = struct{ lat, lon float64 }{
			lat: rand.Float64()*180 - 90,
			lon: rand.Float64()*360 - 180,
		}
	}
	
	// Run benchmark with progress
	var totalResults atomic.Int64
	var queryCount atomic.Int32
	
	start := time.Now()
	
	// Progress updater
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		
		for range ticker.C {
			progress := float64(queryCount.Load()) / float64(numQueries)
			program.Send(progressMsg(progress))
			
			if queryCount.Load() >= int32(numQueries) {
				break
			}
		}
	}()
	
	var wg sync.WaitGroup
	queriesPerWorker := numQueries / numWorkers
	
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		startIdx := w * queriesPerWorker
		endIdx := startIdx + queriesPerWorker
		if w == numWorkers-1 {
			endIdx = numQueries
		}
		
		go func(start, end int) {
			defer wg.Done()
			
			localResults := 0
			for i := start; i < end; i++ {
				c := centers[i]
				center := models.Location{Lat: c.lat, Lon: c.lon}
				results, err := index.QueryRadius(center, searchRadius)
				if err == nil {
					localResults += len(results)
				}
				queryCount.Add(1)
			}
			totalResults.Add(int64(localResults))
		}(startIdx, endIdx)
	}
	
	wg.Wait()
	elapsed := time.Since(start)
	
	completedQueries := queryCount.Load()
	program.Send(stageCompleteMsg{
		stage: stageRadiusSearch,
		stats: benchmarkResult{
			totalQueries:  int64(completedQueries),
			totalTime:     elapsed,
			totalResults:  totalResults.Load(),
			avgQueryTime:  elapsed / time.Duration(completedQueries),
			queriesPerSec: float64(completedQueries) / elapsed.Seconds(),
		},
	})
}

func runNearestNeighbors() {
	
	// Load index
	index := rtree.NewGeoIndex()
	if err := index.LoadFromFile(indexFile); err != nil {
		program.Send(messageMsg(fmt.Sprintf("Error loading index: %v", err)))
		return
	}
	
	numQueries := 1000
	numWorkers := runtime.NumCPU()
	numNeighbors := 10
	
	// Prepare query points
	queryPoints := make([]struct{ lat, lon float64 }, numQueries)
	for i := 0; i < numQueries; i++ {
		queryPoints[i] = struct{ lat, lon float64 }{
			lat: rand.Float64()*180 - 90,
			lon: rand.Float64()*360 - 180,
		}
	}
	
	// Run benchmark with progress
	var totalResults atomic.Int64
	var queryCount atomic.Int32
	
	start := time.Now()
	
	// Progress updater
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		
		for range ticker.C {
			progress := float64(queryCount.Load()) / float64(numQueries)
			program.Send(progressMsg(progress))
			
			if queryCount.Load() >= int32(numQueries) {
				break
			}
		}
	}()
	
	var wg sync.WaitGroup
	queriesPerWorker := numQueries / numWorkers
	
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		startIdx := w * queriesPerWorker
		endIdx := startIdx + queriesPerWorker
		if w == numWorkers-1 {
			endIdx = numQueries
		}
		
		go func(start, end int) {
			defer wg.Done()
			
			localResults := 0
			for i := start; i < end; i++ {
				q := queryPoints[i]
				center := models.Location{Lat: q.lat, Lon: q.lon}
				results := index.NearestNeighbors(center, numNeighbors)
				localResults += len(results)
				queryCount.Add(1)
			}
			totalResults.Add(int64(localResults))
		}(startIdx, endIdx)
	}
	
	wg.Wait()
	elapsed := time.Since(start)
	
	completedQueries := queryCount.Load()
	program.Send(stageCompleteMsg{
		stage: stageNearestNeighbor,
		stats: benchmarkResult{
			totalQueries:  int64(completedQueries),
			totalTime:     elapsed,
			totalResults:  totalResults.Load(),
			avgQueryTime:  elapsed / time.Duration(completedQueries),
			queriesPerSec: float64(completedQueries) / elapsed.Seconds(),
		},
	})
}

func generateRandomPoints(n int) []*models.Point {
	points := make([]*models.Point, n)
	
	numWorkers := runtime.NumCPU()
	batchSize := n / numWorkers
	var wg sync.WaitGroup
	
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		startIdx := w * batchSize
		endIdx := startIdx + batchSize
		if w == numWorkers-1 {
			endIdx = n
		}
		
		go func(start, end int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(start)))
			
			for i := start; i < end; i++ {
				var lat, lon float64
				
				switch r.Intn(5) {
				case 0: // North America
					lat = r.Float64()*30 + 30
					lon = r.Float64()*60 - 120
				case 1: // Europe
					lat = r.Float64()*20 + 40
					lon = r.Float64()*40 - 10
				case 2: // Asia
					lat = r.Float64()*40 + 20
					lon = r.Float64()*80 + 60
				case 3: // South America
					lat = r.Float64()*40 - 50
					lon = r.Float64()*30 - 80
				default: // Random
					lat = r.Float64()*180 - 90
					lon = r.Float64()*360 - 180
				}
				
				points[i] = &models.Point{
					ID: fmt.Sprintf("point_%d", i),
					Location: &models.Location{
						Lat: lat,
						Lon: lon,
					},
				}
			}
		}(startIdx, endIdx)
	}
	
	wg.Wait()
	return points
}

func main() {
	program = tea.NewProgram(initialModel())
	
	if err := program.Start(); err != nil {
		log.Fatal(err)
	}
}