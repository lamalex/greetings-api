package main

import (
	"context"
	"fmt"

	"github.com/kpenfound/greetings-api/.dagger/internal/dagger"
)

type Greetings struct {
	// +private
	Source *dagger.Directory
	// +private
	Repo string
	// +private
	Image string
	// +private
	App string
	// +private
	Backend *dagger.Backend
	// +private
	Frontend *dagger.Frontend
}

func New(
	// +optional
	// +defaultPath="/"
	// +ignore=[".git", "**/node_modules"]
	source *dagger.Directory,
	// +optional
	// +default="github.com/kpenfound/greetings-api"
	repo string,
	// +optional
	// +default="kylepenfound/greetings-api:latest"
	image string,
	// +optional
	// +default="dagger-demo"
	app string,
) *Greetings {
	return &Greetings{
		Source:   source,
		Repo:     repo,
		Image:    image,
		App:      app,
		Backend:  dag.Backend(source.WithoutDirectory("website")),
		Frontend: dag.Frontend(source.Directory("website")),
	}
}

// Run the CI Checks for the project
func (g *Greetings) Check(ctx context.Context) (string, error) {
	// Lint
	lintOut, err := g.Lint(ctx)
	if err != nil {
		return "", err
	}

	// Then Test
	testOut, err := g.Test(ctx)
	if err != nil {
		return "", err
	}

	// Then Build
	_, err = g.Build().Sync(ctx)
	if err != nil {
		return "", err
	}

	return lintOut + "\n\n" + testOut, nil
}

// Run unit tests for the project
func (g *Greetings) Test(ctx context.Context) (string, error) {
	backendResult, err := g.Backend.UnitTest(ctx)
	if err != nil {
		return "", err
	}

	frontendResult, err := g.Frontend.UnitTest(ctx)
	if err != nil {
		return "", err
	}

	return backendResult + "\n" + frontendResult, nil
}

// Lint the Go code in the project
func (g *Greetings) Lint(ctx context.Context) (string, error) {
	backendResult, err := g.Backend.Lint(ctx)
	if err != nil {
		return "", err
	}

	frontendResult, err := g.Frontend.Lint(ctx)
	if err != nil {
		return "", err
	}
	return backendResult + "\n" + frontendResult, nil
}

// Build the backend and frontend for a specified environment
func (g *Greetings) Build() *dagger.Directory {
	return dag.Directory().
		WithFile("/build/greetings-api", g.Backend.Binary()).
		WithDirectory("build/website/", g.Frontend.Build())
}

// Serve the backend and frontend to 8080 and 8081 respectively
func (g *Greetings) Serve() *dagger.Service {
	backendService := g.Backend.Serve()
	frontendService := g.Frontend.Serve()

	return dag.Proxy().
		WithService(backendService, "backend", 8080, 8080).
		WithService(frontendService, "frontend", 8081, 80).
		Service()
}

// Create a GitHub release
func (g *Greetings) Release(ctx context.Context, tag string, ghToken *dagger.Secret) (string, error) {
	// Get build
	build := g.Build()
	// Compress frontend build
	assets := dag.Container().From("alpine:3.18").
		WithDirectory("/assets", build).
		WithWorkdir("/assets/build").
		WithExec([]string{"tar", "czf", "website.tar.gz", "website/"}).
		WithExec([]string{"rm", "-r", "website"}).
		Directory("/assets/build")
	_, _ = assets.Sync(ctx)

	title := fmt.Sprintf("Release %s", tag)
	return dag.GithubRelease().Create(ctx, g.Repo, tag, title, ghToken, dagger.GithubReleaseCreateOpts{Assets: assets})
}

// Debug broken tests
func (g *Greetings) DebugTests(
	ctx context.Context,
	// The model to use to debug debug tests
	// +optional
	model string,
) (string, error) {
	opts := dagger.LlmOpts{}
	if model != "" {
		opts.Model = model
	}
	prompt := dag.CurrentModule().Source().File("prompts/fix_tests_backend.md")

	// Check if backend is broken
	if _, berr := g.Backend.Check(ctx, g.Backend.Source()); berr != nil {
		ws := dag.Workspace(
			g.Backend.Source(),
			g.Backend.AsWorkspaceCheckable(),
		)
		return dag.Llm(opts).
			WithWorkspace(ws).
			WithPromptFile(prompt).
			Workspace().
			Diff(ctx)
	}

	// Check if frontend is broken
	if _, ferr := g.Frontend.Check(ctx, g.Frontend.Source()); ferr != nil {
		ws := dag.Workspace(
			g.Frontend.Source(),
			g.Frontend.AsWorkspaceCheckable(),
		)
		return dag.Llm(opts).
			WithWorkspace(ws).
			WithPromptFile(prompt).
			Workspace().
			Diff(ctx)
	}

	return "", fmt.Errorf("no broken tests found")
}

func (g *Greetings) DebugBrokenTestsPullRequest() {}
