/*
CHUNK HUNTER
	Scans text files for a set of predefined chunks.
	More information: https://github.com/TomDeneire/chunkhunter
*/

package main

import (
	"bufio"
	"encoding/csv"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// FUNCTIONS

// Docs returns slice containing paths to all text files
func Docs(dir string) []string {
	docs := make([]string, 0)
	walkErr := filepath.Walk(dir,
		func(path string, info os.FileInfo, pathErr error) error {
			if pathErr != nil {
				log.Fatalf("Unable to open the input folder: %v", pathErr)
			}
			filename := string(path)
			if strings.HasSuffix(filename, ".txt") {
				docs = append(docs, filename)
			}
			return nil
		})
	if walkErr != nil {
		log.Fatalf("Unable to walk through input folder: %v", walkErr)
	}
	if len(docs) == 0 {
		log.Fatalf("No files found in input folder")
	}
	return docs
}

// ImportDb imports chunks database and returns map
// (using a map eliminates non-unique chunks)
func ImportDb() map[string]int {
	dbMap := make(map[string]int)
	dbFile, openErr := os.Open("chunks.txt")
	if openErr == nil {
		fileScanner := bufio.NewScanner(dbFile)
		for fileScanner.Scan() {
			line := fileScanner.Text()
			chunk := CleanChunks(line)
			dbMap[chunk] = 1
		}
		defer dbFile.Close()
		return dbMap
	}
	if openErr != nil {
		log.Fatalf("Unable to open the chunks database: %v", openErr)
	}
	return nil
}

// CleanChunks preprocesses the chunk strings
func CleanChunks(chunk string) string {
	clean := strings.TrimSpace(chunk)
	lower := strings.ToLower(clean)
	addSpace := " " + lower // space added for chunk unit otherwise you'll get hits like 'wAS ALWAYS
	return addSpace
}

// Conv converts and preprocesses input texts
func Conv(file string) string {
	inputBytes, inputErr := ioutil.ReadFile(file)
	if inputErr != nil {
		log.Printf("Unable to open the file: %v", inputErr)
	}
	return " " + string(inputBytes) // added space to allow first word to be a chunk
}

// APPLICATION

func main() {

	// Initialize
	// start := time.Now()
	chunks := ImportDb()
	osSep := string(os.PathSeparator)
	os.MkdirAll("output", 0777)
	// Loop over files
	files := Docs("input")
	for _, file := range files {
		pathSplit := strings.Split(file, osSep)
		fileName := strings.Replace(pathSplit[1], ".", "-", -1)
		inputText := Conv(file)
		words := len(strings.Fields(inputText))
		inputCheck := strings.ToLower(inputText)
		inputHTML := inputText
		chunkTotal := 0
		records := [][]string{
			{"chunk", "length", "frequency"}}
		chunkCount := make(map[string]int)
		for chunk := range chunks {
			if strings.Contains(inputCheck, chunk) {
				chunkFreq := strings.Count(inputCheck, chunk)
				// html
				// regex version (slow but complete)
				re, _ := regexp.Compile("(?i)" + chunk)
				inputHTML = re.ReplaceAllStringFunc(inputHTML, func(arg string) string {
					return "<span style=\"background-color: #FFFF00\">" + arg + "</span>"
				})
				// replace version (fast but incomplete)
				// allUpper := strings.ToUpper(chunk)
				// capitalized := strings.ToUpper(string(chunk[1])) + chunk[2:len(chunk)]
				// matches := []string{chunk, allUpper, capitalized}
				// for _, match := range matches {
				// 	inputHTML = strings.Replace(inputHTML, match, "<span style=\"background-color: #FFFF00\">"+match+"</span>", -1)
				// }
				// csv
				chunkString := strings.TrimSpace(chunk) // remove space added to define chunk unit
				record := []string{chunkString, strconv.Itoa(len(chunk)), strconv.Itoa(chunkFreq)}
				records = append(records, record)
				// count
				chunkCount[chunkString] = chunkFreq
				chunkTotal = chunkTotal + (len(strings.Split(chunkString, " ")) * chunkFreq)
			}
		}
		// Write html file
		htmlFile, htmlErr := os.Create("output" + osSep + "result_" + fileName + ".html")
		if htmlErr != nil {
			log.Printf("Unable to write result file: %v", htmlErr)
			return
		}
		defer htmlFile.Close()
		htmlFile.WriteString(inputHTML)
		// Write csv file
		csvFile, csvError := os.Create("output" + osSep + "stats_" + fileName + ".csv")
		if csvError != nil {
			log.Printf("Cannot create file: %v", csvError)
		}
		defer csvFile.Close()
		w := csv.NewWriter(csvFile)
		w.WriteAll(records)
		if err := w.Error(); err != nil {
			log.Fatalln("error writing csv:", err)
		}
		// Write statistics file
		percent := float64(chunkTotal) / float64(words) * 100
		statFile, statErr := os.Create("output" + osSep + "stats_" + fileName + ".txt")
		if statErr != nil {
			log.Printf("Unable to write statistics file: %v", statErr)
			return
		}
		defer statFile.Close()
		statFile.WriteString("--------------------------------------------------------\n")
		statFile.WriteString("Chunk Hunter statistics for " + file + "\n")
		statFile.WriteString("--------------------------------------------------------\n\n")
		stats := "Frequency: " + strconv.Itoa(chunkTotal) + " chunkwords / " + strconv.Itoa(words) + " words = " + strconv.FormatFloat(percent, 'f', 2, 64) + "%\n\n"
		statFile.WriteString(stats)
		number := "Text checked against " + strconv.Itoa(len(chunks)) + " registered combinations\n\n"
		statFile.WriteString(number)
	}
	// t := time.Now()
	// fmt.Println(t.Sub(start))
}
