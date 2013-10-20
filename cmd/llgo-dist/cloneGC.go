
package main

import (
	"go/build"
	"log"
	"os"
	"path"
	"os/exec"
)

// The values are subdirectories of the main gc directory.
var gcRepositories = map[string]string {
	"git://github.com/ivmai/bdwgc.git" : "",
	"git://github.com/ivmai/libatomic_ops.git" : "libatomic_ops",
}

func gitClone(repo string, targetDir string) error {
	cmd := exec.Command("git", "clone", repo, targetDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func checkGitRepository(directory string) (repoExists bool, err error) {
	// Use --porcelain even though we are not actually parsing the output.
	// We _should_ check the exit code, but currently there's no portable way to do this.
	// So just check if the output contains no lines, which means everything is fine.
	
	repoExists = false
	statusOutput, err := command("git", "status", "--porcelain").CombinedOutput()
	if err == nil {
		repoExists = true
		if len(statusOutput) > 0 {
			// No output means the repository does not exist or is corrupted.
			if err = os.RemoveAll(directory); err == nil {
				repoExists = false
			} else {
				log.Printf("Wanted to clone GC into %s, but failed to delete the directory.\n", directory)
			}
		}
	}
	return
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func cloneGarbageCollector() error {
	pkg, err := build.Import(llgoBuildPath, "", build.FindOnly)
	if err != nil {
		return err
	}
	gcDir := path.Join(pkg.Dir, "..", "..", "pkg", "runtime", "gc")
	doClone := !fileExists(gcDir)
	if !doClone {
		repoExists, err := checkGitRepository(gcDir)
		if err != nil {
			return err
		}
		doClone = !repoExists
	}
	if doClone {
		log.Println("Cloning garbage collector repositories...")
		for repo, targetDir := range gcRepositories {
			if err := gitClone(repo, path.Join(gcDir, targetDir)); err != nil {
				return err
			}
		}
	} else {
		log.Println("Garbage collector repositories already cloned.")
	}
	return nil
}
