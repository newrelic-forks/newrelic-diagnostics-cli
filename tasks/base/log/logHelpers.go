package log

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	log "github.com/newrelic/newrelic-diagnostics-cli/logger"
	"github.com/newrelic/newrelic-diagnostics-cli/tasks"
	baseConfig "github.com/newrelic/newrelic-diagnostics-cli/tasks/base/config"
)

var logFilenamePatterns = []string{"newrelic_agent.*[.]log$",
	"newrelic-daemon[.]log$",
	"php_agent[.]log$",
	"newrelic-python-agent[.]log$",
	"NewRelic[.]Profiler.*[.]log$",
	"newrelic-infra.*[.]log$",
	"synthetics-minion[.]log$",
	"nrinstall-\\d{8}-\\d{6}-\\d{1,}[.]tar$",
}

var secureLogFilenamePatterns = []string{"docker[.]log$",
	"syslog$",
}

var logSysProps = []string{
	"-Dnewrelic.logfile",              //EX: Dnewrelic.logfile=/opt/newrelic/java/logs/newrelic/somenewnameformylogs.log
	"-Dnewrelic.config.log_file_name", //JAVA_OPTS="${JAVA_OPTS} -Dnewrelic.config.log_file_name=NR12345.log"
	"-Dnewrelic.config.log_file_path", //JAVA_OPTS="${JAVA_OPTS} -Dnewrelic.config.log_file_path=/srv/common-api-gateway"
}

var logDirSysProp = "-Dnewrelic.config.log_file_path"
var logNameSysProp = "-Dnewrelic.config.log_file_name"
var logFullPathSysProp = "-Dnewrelic.logfile"

var logEnvVars = []string{
	"NRIA_LOG_FILE", // Infra agent
	"NEW_RELIC_LOG", //Java, Node and python agent paths
}

var keysInConfigFile = map[string][]string{
	"fullpaths": []string{
		"log_file",                //Python: "tmp/newrelic-python-agent.log"
		"newrelic.daemon.logfile", //PHP daemon default val: "/var/log/newrelic/newrelic-daemon.log"
		"newrelic.logfile",        //PHP: "/var/log/newrelic/newrelic-daemon.log",
		"logging.filepath",        // Node: "node/app/newrelic_agent.log"
	},
	"filenames": []string{
		"log_file_name", //Java and Ruby: "newrelic_agent.log"
		"-fileName",     //.NET: "FILENAME.log"
	},
	"directories": []string{
		"log_file_path", //Java, ruby: "/Users/shuayhuaca/Desktop/"
		"-directory",    //.NET "PATH\TO\LOG\DIRECTORY"
	},
	//"log.directory", //NET: how do we parse XML files?
}

type LogElement struct {
	FileName           string
	FilePath           string
	Source             LogSourceData
	IsSecureLocation   bool
	CanCollect         bool
	ReasonToNotCollect string
}

type LogSourceData struct {
	FoundBy  string
	KeyVals  map[string]string
	FullPath string
}

var (
	logPathDefaultSource         = "Found by looking at New Relic default paths"
	logPathConfigFileSource      = "Found by looking at values in New Relic config file settings"
	logPathEnvVarSource          = "Found by looking at New Relic environment variables"
	logPathSysPropSource         = "Found by looking at JVM arguments"
	logPathDiagnosticsFlagSource = "Found by looking at the path defined by user through the " + tasks.ThisProgramFullName + " command line flag: logpath"
)

func collectFilePaths(envVars map[string]string, configElements []baseConfig.ValidateElement, foundSysProps map[string]string) []LogElement {
	var paths []string
	currentPath, err := os.Getwd()
	if err != nil {
		log.Info("Error reading local working directory")
	}
	paths = append(paths, currentPath)

	if runtime.GOOS == "windows" {
		sysProgramFiles := envVars["ProgramFiles"]
		sysProgramData := envVars["ProgramData"]
		sysAppData := envVars["APPDATA"]

		paths = append(paths, sysProgramFiles+`\New Relic`)
		paths = append(paths, sysProgramData+`\New Relic\.NET Agent\Logs`)

		paths = append(paths, sysProgramFiles+`\New Relic\newrelic-infra\newrelic-infra.log`) //Windows, agent version 1.0.752 or lower
		paths = append(paths, sysProgramData+`\New Relic\newrelic-infra\newrelic-infra.log`)  //Windows, agent version 1.0.944 or higher

		//new infra logs (added if statment becasue I am unsure if the envvar will always point to the Roaming folder and not local or localLow )
		//Windows, agent version 1.0.775 to 1.0.944
		if strings.HasSuffix(sysAppData, "Roaming") {
			paths = append(paths, sysAppData+`\New Relic\newrelic-infra`)
		} else if strings.HasSuffix(sysAppData, "Local") {

			paths = append(paths, strings.TrimRight(sysAppData, "Local")+`Roaming\New Relic\newrelic-infra`)

		} else if strings.HasSuffix(sysAppData, "LocalLow") {
			paths = append(paths, strings.TrimRight(sysAppData, "LocalLow")+`Roaming\New Relic\newrelic-infra`)

		} else {
			paths = append(paths, sysAppData+`\Roaming\New Relic\newrelic-infra`)
		}

	} else {
		/*
			While the directories listed here will be walked, it is important to add any directory
			where a NR log file is expected, as only paths in this slice (no subdirectories)
			will be resolved from symbolic links. Matches will be deduped by tasks.FindFiles
		*/
		paths = append(paths, "/tmp")                                     //For Python Agent log and PHP installation log
		paths = append(paths, "/var/log")                                 //For Syn Minion and Infra
		paths = append(paths, "/var/log/newrelic")                        // For PHP agent and daemon log
		paths = append(paths, "/usr/local/newrelic-netcore20-agent/logs") // for dotnetcore
	}
	/*
		Look for new relic log files in this order:
		env vars
		system properties
		settings defined in new relic config files
		standard locations, only if we didn't find any of the above
	*/
	var logFilesFound []LogElement //logEnvVars contains OS-agnostic Environment variables (full path to the log or only log filename) Any filepaths found in env vars will be automatically collected without prompting the user for permissions to collect file
	unmatchedDirKeyToVal := make(map[string]string)
	unmatchedFilenameKeyToVal := make(map[string]string)

	//collect log paths from new relic environment variables
	for _, logEnvVar := range logEnvVars {
		logPath, isPresent := envVars[logEnvVar]
		if isPresent {
			//isIncompletePath represent those path value founds that did not provides full path to log file but only a directory path or a filename. Those incomplete paths are getting collected in a map called unmatchedDirKeyToVal or unmatchedFilenameKeyToVal
			logElement, isIncompletePath := getLogPathFromEnvVar(logPath, logEnvVar, unmatchedFilenameKeyToVal)
			if !isIncompletePath {
				logFilesFound = append(logFilesFound, logElement)
			}
		}
	}

	//collect log paths from new relic JVM arguments
	if len(foundSysProps) > 0 {
		logFullPathSysPropVal, isPresent := foundSysProps[logFullPathSysProp]
		if isPresent {
			logElement := getLogPathFromSysProp(logFullPathSysProp, logFullPathSysPropVal)
			logFilesFound = append(logFilesFound, logElement)
		} else {
			//These system properties(-Dnewrelic.config.log_file_path and -Dnewrelic.config.log_file_name) mimic the behavior these config file settings(log_file_path and log_file_name). But config file settings will take precedence over this type of system props
			logElement, isIncompletePath := getLogPathFromConfigSysProps(foundSysProps, unmatchedDirKeyToVal, unmatchedFilenameKeyToVal)
			if !isIncompletePath {
				logFilesFound = append(logFilesFound, logElement)
			}
		}
	}

	//collect log paths from values found in new relic config files settings
	if len(configElements) > 0 {
		logElements := getLogPathsFromConfigFile(configElements, unmatchedDirKeyToVal, unmatchedFilenameKeyToVal)
		if len(logElements) > 0 {
			logFilesFound = append(logFilesFound, logElements...)
		}
	}

	if len(logFilesFound) < 1 {
		if len(unmatchedDirKeyToVal) > 0 && len(unmatchedFilenameKeyToVal) > 0 {
			logElements := getLogPathsFromCombinedUnmatchedDirFilename(unmatchedDirKeyToVal, unmatchedFilenameKeyToVal)
			if len(logElements) > 0 {
				logFilesFound = append(logFilesFound, logElements...)
				return logFilesFound
			}
		}
		//Last attempt to find full path by looking for given filename in current path or by looking for logFilenamePatterns inside of a given directory
		logElements := getLogPathsFromCurrentDirOrNamePatters(unmatchedDirKeyToVal, unmatchedFilenameKeyToVal, currentPath)
		if len(logElements) > 0 {
			logFilesFound = append(logFilesFound, logElements...)
			return logFilesFound
		}
		//If at this point we haven't found any log files, then we can make effort of seraching in all standard new relic locations using all standard new relic log file names
		backupLogElements := getLogPathsFromStandardLocations(paths)
		logFilesFound = append(logFilesFound, backupLogElements...)
	}

	return logFilesFound
}

func getLogPathsFromCurrentDirOrNamePatters(unmatchedDirKeyToVal, unmatchedFilenameKeyToVal map[string]string, currentPath string) []LogElement {
	var logElements []LogElement
	if len(unmatchedDirKeyToVal) > 0 {
		for dirKey, dirVal := range unmatchedDirKeyToVal {
			logPaths := findLogFiles(logFilenamePatterns, dirVal)
			fmt.Println(len(logPaths) == 0)
			if len(logPaths) > 0 {
				for _, fullPath := range logPaths {
					dir, fileName := filepath.Split(fullPath)
					logElements = append(logElements, LogElement{
						FileName: fileName,
						FilePath: dir,
						Source: LogSourceData{
							FoundBy: fmt.Sprintf("Found by looking for standard names for New Relic log files in the provided directory value %s for the key %s", dirVal, dirKey),
							KeyVals: map[string]string{
								dirKey: dirVal,
							},
							FullPath: fullPath,
						},
						IsSecureLocation: false,
						CanCollect:       true,
					})

				}
			}
		}
	}

	if len(unmatchedFilenameKeyToVal) > 0 {
		for filenameKey, filenameVal := range unmatchedFilenameKeyToVal {
			logPaths := tasks.FindFiles([]string{filenameVal}, []string{currentPath})
			if len(logPaths) > 0 {
				for _, fullPath := range logPaths {
					dir, fileName := filepath.Split(fullPath)
					logElements = append(logElements, LogElement{
						FileName: fileName,
						FilePath: dir,
						Source: LogSourceData{
							FoundBy: fmt.Sprintf("Found by looking in the current directory for the provided log filename(%s) through the key %s", filenameVal, filenameKey),
							KeyVals: map[string]string{
								filenameKey: filenameVal,
							},
							FullPath: fullPath,
						},
						IsSecureLocation: false,
						CanCollect:       true,
					})
				}
			}
		}
	}
	return logElements
}

func getLogPathsFromCombinedUnmatchedDirFilename(unmatchedDirKeyToVal, unmatchedFilenameKeyToVal map[string]string) []LogElement {
	var logElements []LogElement

	for dirKey, dirVal := range unmatchedDirKeyToVal {
		pathsToFiles := getFilesFromDir(dirVal)
		for _, pathToFile := range pathsToFiles {
			for filenameKey, filenameVal := range unmatchedFilenameKeyToVal {
				regex := regexp.MustCompile(filenameVal)
				if regex.MatchString(pathToFile) {
					logElements = append(logElements, LogElement{
						FileName: filenameVal,
						FilePath: dirVal,
						Source: LogSourceData{
							FoundBy: fmt.Sprintf("Found by looking for a file named %s in the directory path %s", filenameVal, dirVal),
							KeyVals: map[string]string{
								dirKey:      dirVal,
								filenameKey: filenameVal,
							},
							FullPath: pathToFile,
						},
						IsSecureLocation: false,
						CanCollect:       true,
					})
				}
			}
		}
	}

	return logElements
}

func getLogPathsFromStandardLocations(paths []string) []LogElement {
	var logElements []LogElement
	//findFiles will return a full path that include filename
	fileLocations := tasks.FindFiles(logFilenamePatterns, paths)
	if len(fileLocations) > 0 {
		for _, fileLocation := range fileLocations {
			dir, fileName := filepath.Split(fileLocation)
			logElements = append(logElements, LogElement{
				FileName: fileName,
				FilePath: dir,
				Source: LogSourceData{
					FoundBy:  logPathDefaultSource,
					FullPath: fileLocation,
				},
				IsSecureLocation: false,
				CanCollect:       true,
			})
		}
	}

	secureFileLocations := tasks.FindFiles(secureLogFilenamePatterns, paths)
	if len(secureFileLocations) > 0 {
		for _, fileLocation := range secureFileLocations {
			dir, fileName := filepath.Split(fileLocation)
			logElements = append(logElements, LogElement{
				FileName: fileName,
				FilePath: dir,
				Source: LogSourceData{
					FoundBy:  logPathDefaultSource,
					FullPath: fileLocation,
				},
				IsSecureLocation: true,
				CanCollect:       true,
			})
		}
	}

	return logElements
}

func getLogPathFromSysProp(sysPropKey, sysPropVal string) LogElement {
	dir, fileName := filepath.Split(sysPropVal)
	return LogElement{
		FileName: fileName,
		FilePath: dir,
		Source: LogSourceData{
			FoundBy: logPathSysPropSource,
			KeyVals: map[string]string{
				sysPropKey: sysPropVal,
			},
			FullPath: sysPropVal,
		},
		IsSecureLocation: false,
		CanCollect:       true,
	}
}

func getLogPathFromEnvVar(logPath string, logEnvVar string, unmatchedFilenameKeyToVal map[string]string) (LogElement, bool) {
	if logPath == "stdout" || logPath == "stderr" {
		return LogElement{
			Source: LogSourceData{
				FoundBy: logPathEnvVarSource,
				KeyVals: map[string]string{
					logEnvVar: logPath,
				},
				FullPath: logPath,
			},
			IsSecureLocation:   false,
			CanCollect:         false,
			ReasonToNotCollect: tasks.ThisProgramFullName + " cannot collect logs that have been set to STDOUT OR STDERR",
		}, false
	}

	dir, fileName := filepath.Split(logPath)
	if len(dir) > 0 { //path is a directory or a fullpath that includes filename
		return LogElement{
			FileName: fileName,
			FilePath: dir,
			Source: LogSourceData{
				FoundBy: logPathEnvVarSource,
				KeyVals: map[string]string{
					logEnvVar: logPath,
				},
				FullPath: logPath,
			},
			IsSecureLocation: false,
			CanCollect:       true,
		}, false
	}
	unmatchedFilenameKeyToVal[logEnvVar] = fileName
	return LogElement{}, true
}

func getLogPathFromConfigSysProps(configSysProps, unmatchedDirKeyToVal, unmatchedFilenameKeyToVal map[string]string) (LogElement, bool) {
	dir, isDirPresent := configSysProps[logDirSysProp]
	filename, isNamePresent := configSysProps[logNameSysProp]

	if isDirPresent && isNamePresent {
		fullpath := filepath.Join(dir, filename)
		return LogElement{
			Source: LogSourceData{
				FoundBy:  logPathSysPropSource,
				KeyVals:  configSysProps,
				FullPath: fullpath,
			},
			IsSecureLocation: false,
			CanCollect:       true,
		}, false
	}

	if isDirPresent {
		unmatchedDirKeyToVal[logDirSysProp] = dir
		return LogElement{}, true
	}

	unmatchedFilenameKeyToVal[logNameSysProp] = filename
	return LogElement{}, true
}

func getLogPathsFromConfigFile(configElements []baseConfig.ValidateElement, unmatchedDirKeyToVal, unmatchedFilenameKeyToVal map[string]string) []LogElement {
	configMap := make(map[string]string)
	var logElements []LogElement

	for _, configFile := range configElements {
		var fullPath, filename, dir, configKey string
		//search for nr log keys that contain fullPath to the logs
		for _, v := range keysInConfigFile["fullPaths"] {
			foundKeys := configFile.ParsedResult.FindKey(v)
			for _, key := range foundKeys {
				val := key.Value() //"myapp/newrelic_agent.log"
				if len(val) > 0 {
					fullPath = val
					configKey = key.Key
					configMap[configKey] = fullPath
					break //we should only grab one log path per config file
				}
			}
		}

		for _, v := range keysInConfigFile["filenames"] {
			foundKeys := configFile.ParsedResult.FindKey(v)
			for _, key := range foundKeys {
				val := key.Value()
				if len(val) > 0 {
					filename = val
					configKey = key.Key
					configMap[configKey] = filename
					break //we should only grab one log path per config file
				}
			}
		}

		for _, v := range keysInConfigFile["directories"] {
			foundKeys := configFile.ParsedResult.FindKey(v)
			for _, key := range foundKeys {
				val := key.Value()
				if len(val) > 0 {
					dir = val
					configKey = key.Key
					configMap[configKey] = dir
					break //we should only grab one log path per config file
				}
			}
		}

		if len(fullPath) > 0 {
			dir, fileName := filepath.Split(fullPath)
			logElements = append(logElements, LogElement{
				FileName:         fileName,
				FilePath:         dir,
				Source:           LogSourceData{FoundBy: logPathConfigFileSource, KeyVals: configMap, FullPath: fullPath},
				IsSecureLocation: false,
				CanCollect:       true,
			})
		}
		if len(dir) > 0 && len(filename) > 0 {
			fullPath := filepath.Join(dir, filename) //we are doing this instead of dir+filename because dir may not have a trailing slash at the end
			logElements = append(logElements, LogElement{
				FileName:         filename,
				FilePath:         dir,
				Source:           LogSourceData{FoundBy: logPathConfigFileSource, KeyVals: configMap, FullPath: fullPath},
				IsSecureLocation: false,
				CanCollect:       true,
			})
		} else {
			if len(dir) > 0 {
				unmatchedDirKeyToVal[configKey] = dir
			}
			if len(filename) > 0 {
				unmatchedFilenameKeyToVal[configKey] = filename
			}
		}
	}

	return logElements
}

//Similar to tasks.FindFiles except that it does not traverse through sub-directories to find those filenames provided
//nolint
func findLogFiles(patterns []string, dir string) []string {
	pathsToFiles := getFilesFromDir(dir)

	// map to automatically dedupe file matches
	foundLogFiles := make(map[string]struct{})
	for _, file := range pathsToFiles {
		// loop through pattern list and add files that match to our string array
		for _, pattern := range patterns {
			regex := regexp.MustCompile(pattern)
			if regex.MatchString(file) {
				foundLogFiles[file] = struct{}{} // empty struct is smallest memory footprint
			}
		}
	}
	var uniqueFoundFiles []string
	for fileLocation := range foundLogFiles {
		uniqueFoundFiles = append(uniqueFoundFiles, fileLocation)
	}
	return uniqueFoundFiles
}

func getFilesFromDir(dir string) []string {
	var potentialLogFiles []string
	f, err := os.Open(dir)
	if err != nil {
		log.Debug(err)
	}
	files, err := f.Readdir(-1)
	f.Close()
	if err != nil {
		log.Debug(err)
	}

	for _, file := range files {
		if !file.IsDir() && !(strings.HasPrefix(file.Name(), ".")) {
			fullPath := filepath.Join(dir, file.Name())
			potentialLogFiles = append(potentialLogFiles, fullPath)
		}
	}
	return potentialLogFiles
}
