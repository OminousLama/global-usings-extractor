package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	cp "github.com/otiai10/copy"
)

var version = "undefined"
var metaBuildTime = "undefined"
var metaBuilderOS = "undefined"
var metaBuilderArch = "undefined"

const (
	DIRTYPE_SOLUTION    = 2
	DIRTYPE_PROJECT     = 1
	DIRTYPE_UNSUPPORTED = 0
)

func main() {
	//#region CLI param definitions
	showVersion := flag.Bool("v", false, "Show version information")
	flag.BoolVar(showVersion, "version", false, "Show version information")

	dir := flag.String("d", "", "Project or solution directory")
	disableDirectoryIsolation := flag.Bool("disable-isolation", false, "(DANGEROUS, not recommended!) Disables copying the target files to a temporary working directory.")
	//guClassName := flag.String("c", "GlobalUsings", "Name of the global usings class")

	flag.Parse()
	//#endregion

	//#region Parse CLI params
	//#region Version info
	if *showVersion {
		fmt.Println("guext version info:")
		fmt.Println("- Version:", version)
		fmt.Println("- Build time:", metaBuildTime)
		fmt.Println("- Builder OS:", metaBuilderOS)
		fmt.Println("- Builder Arch:", metaBuilderArch)
	}

	if *dir == "" {
		fmt.Println("You need to specify a project or solution directory.")
		return
	}
	//#endregion
	//#endregion

	//#region Get project root dirs

	projectDirs, err := findDirectoriesWithCSProj(*dir)
	if err != nil {
		fmt.Println("Error getting project directories: ", err)
		return
	}

	if len(projectDirs) == 0 {
		fmt.Println("The specified directory does not contain any .csproj files.")
		return
	}

	workingDir := *dir
	if !*disableDirectoryIsolation {
		workingDir = createWorkingDirectory(*dir)

		// Re-index after moving to working directory
		projectDirs, err = findDirectoriesWithCSProj(workingDir)
	}

	//#endregion

	//#region Transform files

	for _, value := range projectDirs {
		fmt.Printf("Processing '%s'...\n", value)

		globalUsings, err := processCSFiles(value)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		err = createGlobalUsingsFile(value, globalUsings)
		if err != nil {
			fmt.Println("Error creating GlobalUsings file:", err)
			return
		}
	}

	//#endregion

}

func createWorkingDirectory(sourceDir string) (workingDir string) {
	uuidObj := uuid.New()
	workingDirPath := filepath.Join(sourceDir, "../.guext-tmp", fmt.Sprint(uuidObj.String()))

	err := os.MkdirAll(workingDirPath, os.ModePerm)
	if err != nil {
		fmt.Println("Error creating directory:", err)
		return
	}

	cp.Copy(sourceDir, workingDirPath)

	return workingDirPath
}

func findDirectoriesWithCSProj(rootDir string) ([]string, error) {
	var result []string

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			csprojFiles, err := filepath.Glob(filepath.Join(path, "*.csproj"))
			if err != nil {
				return err
			}
			if len(csprojFiles) > 0 {
				result = append(result, path)
			}
		}
		return nil
	})

	return result, err
}

func processCSFiles(rootDir string) ([]string, error) {
	var globalUsings []string

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if filepath.Ext(path) == ".cs" {
			usings, err := extractAndRemoveUsings(path)
			if err != nil {
				return err
			}
			globalUsings = append(globalUsings, usings...)
		}
		return nil
	})

	return globalUsings, err
}

func extractAndRemoveUsings(filePath string) ([]string, error) {
	file, err := os.OpenFile(filePath, os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var lines []string
	var usings []string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(strings.TrimSpace(line), "using ") {
			usings = append(usings, line)
			// Skip this line to effectively remove the using statement
			continue
		}

		lines = append(lines, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Write the modified lines back to the file
	if err := file.Truncate(0); err != nil {
		return nil, err
	}
	file.Seek(0, 0)

	writer := bufio.NewWriter(file)
	for _, l := range lines {
		fmt.Fprintln(writer, l)
	}
	writer.Flush()

	return usings, nil
}

func createGlobalUsingsFile(rootDir string, usings []string) error {
	globalUsingsPath := filepath.Join(rootDir, "GlobalUsings.cs")
	file, err := os.OpenFile(globalUsingsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, u := range usings {
		fmt.Fprintln(file, "global "+u)
	}

	// Deduplicate (TODO: Make this workflow more efficient / avoid dupes in the first place)
	removeDuplicatesFromFile(globalUsingsPath)

	return nil
}

func removeDuplicatesFromFile(filePath string) error {

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	uniqueLines := make(map[string]struct{})

	var orderedLines []string

	// Read the file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if _, exists := uniqueLines[line]; !exists {
			uniqueLines[line] = struct{}{}
			orderedLines = append(orderedLines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	// Open file for writing truncating existing content
	outputFile, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer outputFile.Close()

	// Write unique lines to file
	for _, line := range orderedLines {
		fmt.Fprintln(outputFile, line)
	}

	return nil
}
