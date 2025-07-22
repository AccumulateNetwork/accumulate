# CI/CD Integration Guide

## Table of Contents

1. [Overview](#overview)
2. [Quick Start](#quick-start)
3. [GitLab CI Configuration](#gitlab-ci-configuration)
4. [GitHub Actions Configuration](#github-actions-configuration)
5. [Test Pipeline Design](#test-pipeline-design)
6. [Performance Testing in CI](#performance-testing-in-ci)
7. [Artifact Management](#artifact-management)
8. [Quality Gates](#quality-gates)
9. [Monitoring and Alerts](#monitoring-and-alerts)
10. [Best Practices](#best-practices)

## Overview

This guide covers integrating the Accumulate Network test suite into CI/CD pipelines, ensuring automated testing, quality assurance, and deployment workflows.

### CI/CD Goals

- **Automated Testing**: Run comprehensive test suite on every change
- **Quality Assurance**: Enforce code quality and coverage standards
- **Performance Monitoring**: Track performance regressions
- **Fast Feedback**: Provide quick feedback to developers
- **Reliable Deployments**: Ensure only tested code reaches production

## Quick Start

### Basic Pipeline Structure

```yaml
# .gitlab-ci.yml or .github/workflows/test.yml
stages:
  - validate
  - test
  - performance
  - security
  - deploy

variables:
  GO_VERSION: "1.21"
  POSTGRES_VERSION: "15"
  REDIS_VERSION: "7"
```

### Essential Test Commands

```bash
# Unit tests with coverage
go test -race -coverprofile=coverage.out ./...

# Integration tests
go test -tags=integration -timeout=30m ./test/e2e/...

# Performance benchmarks
go test -bench=. -benchmem ./...

# Security scanning
gosec ./...
```

## GitLab CI Configuration

### Complete Pipeline Configuration

```yaml
# .gitlab-ci.yml
image: golang:1.21

variables:
  GO_VERSION: "1.21"
  POSTGRES_DB: accumulate_test
  POSTGRES_USER: test
  POSTGRES_PASSWORD: test
  REDIS_URL: redis://redis:6379
  CGO_ENABLED: 0
  GOOS: linux
  GOARCH: amd64

stages:
  - validate
  - test
  - performance
  - security
  - build
  - deploy

# Cache configuration
.go-cache: &go-cache
  cache:
    key: "${CI_JOB_NAME}-${CI_COMMIT_REF_SLUG}"
    paths:
      - .cache/go-build/
      - .cache/go-mod/
    policy: pull-push

before_script:
  - mkdir -p .cache/go-build .cache/go-mod
  - export GOCACHE=$PWD/.cache/go-build
  - export GOMODCACHE=$PWD/.cache/go-mod
  - go version
  - go mod download

# Validation stage
lint:
  <<: *go-cache
  stage: validate
  script:
    - go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
    - golangci-lint run --timeout=10m
  artifacts:
    reports:
      junit: lint-report.xml
    paths:
      - lint-report.xml
    expire_in: 1 week

format-check:
  <<: *go-cache
  stage: validate
  script:
    - gofmt -l . | tee format-issues.txt
    - test ! -s format-issues.txt
  artifacts:
    paths:
      - format-issues.txt
    expire_in: 1 week
    when: on_failure

# Test stage
unit-tests:
  <<: *go-cache
  stage: test
  services:
    - postgres:15
    - redis:7
  variables:
    POSTGRES_HOST_AUTH_METHOD: trust
  script:
    - go test -race -coverprofile=coverage.out -covermode=atomic ./...
    - go tool cover -html=coverage.out -o coverage.html
    - go tool cover -func=coverage.out | tail -1
  coverage: '/total:\s+\(statements\)\s+(\d+\.\d+)%/'
  artifacts:
    reports:
      junit: test-report.xml
      coverage_report:
        coverage_format: cobertura
        path: coverage.xml
    paths:
      - coverage.out
      - coverage.html
      - test-report.xml
    expire_in: 1 week

integration-tests:
  <<: *go-cache
  stage: test
  services:
    - postgres:15
    - redis:7
  variables:
    POSTGRES_HOST_AUTH_METHOD: trust
    ACC_TEST_TIMEOUT: 30m
  script:
    - go test -tags=integration -timeout=30m -v ./test/e2e/...
  artifacts:
    reports:
      junit: integration-report.xml
    paths:
      - integration-report.xml
    expire_in: 1 week
  timeout: 45m

simulator-tests:
  <<: *go-cache
  stage: test
  script:
    - go test -timeout=20m -v ./test/simulator/...
  artifacts:
    reports:
      junit: simulator-report.xml
    paths:
      - simulator-report.xml
    expire_in: 1 week
  timeout: 30m

# Performance stage
benchmark-tests:
  <<: *go-cache
  stage: performance
  script:
    - go test -bench=. -benchmem -benchtime=10s ./... | tee benchmark.txt
    - go install golang.org/x/perf/cmd/benchstat@latest
    - benchstat benchmark.txt
  artifacts:
    paths:
      - benchmark.txt
    expire_in: 1 month
  only:
    - main
    - develop
    - merge_requests

load-tests:
  <<: *go-cache
  stage: performance
  services:
    - postgres:15
    - redis:7
  variables:
    POSTGRES_HOST_AUTH_METHOD: trust
  script:
    - go build -o accumulated ./cmd/accumulated
    - ./accumulated --config=test/configs/ci.toml &
    - sleep 10
    - go run ./test/cmd/load -server=http://localhost:8080 -duration=60s -transactions=1000
  artifacts:
    paths:
      - load-test-results.json
    expire_in: 1 month
  timeout: 10m
  only:
    - main
    - develop

# Security stage
security-scan:
  <<: *go-cache
  stage: security
  script:
    - go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest
    - gosec -fmt json -out gosec-report.json ./...
  artifacts:
    reports:
      sast: gosec-report.json
    paths:
      - gosec-report.json
    expire_in: 1 week
  allow_failure: true

dependency-check:
  <<: *go-cache
  stage: security
  script:
    - go install github.com/sonatypecommunity/nancy@latest
    - go list -json -deps ./... | nancy sleuth
  allow_failure: true

# Build stage
build:
  <<: *go-cache
  stage: build
  script:
    - go build -ldflags="-w -s" -o accumulated ./cmd/accumulated
    - go build -ldflags="-w -s" -o analyze ./tools/cmd/analyze
  artifacts:
    paths:
      - accumulated
      - analyze
    expire_in: 1 week

# Deploy stage (example)
deploy-staging:
  stage: deploy
  script:
    - echo "Deploying to staging environment"
    - ./deploy-scripts/staging.sh
  environment:
    name: staging
    url: https://staging.accumulate.io
  only:
    - develop
  when: manual

deploy-production:
  stage: deploy
  script:
    - echo "Deploying to production environment"
    - ./deploy-scripts/production.sh
  environment:
    name: production
    url: https://accumulate.io
  only:
    - main
  when: manual
```

### GitLab-Specific Features

```yaml
# Performance monitoring
performance-monitoring:
  stage: performance
  script:
    - go test -bench=. -benchmem ./... | tee benchmark.txt
    - python3 scripts/parse-benchmarks.py benchmark.txt > metrics.json
  artifacts:
    reports:
      performance: metrics.json

# Code quality
code-quality:
  stage: validate
  script:
    - golangci-lint run --out-format code-climate > gl-code-quality-report.json
  artifacts:
    reports:
      codequality: gl-code-quality-report.json
```

## GitHub Actions Configuration

### Complete Workflow Configuration

```yaml
# .github/workflows/test.yml
name: Test Suite

on:
  push:
    branches: [ main, develop ]
  pull_request:
    branches: [ main, develop ]

env:
  GO_VERSION: '1.21'
  POSTGRES_VERSION: '15'
  REDIS_VERSION: '7'

jobs:
  validate:
    name: Validation
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v4
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: ${{ env.GO_VERSION }}
        cache: true
    
    - name: Lint
      uses: golangci/golangci-lint-action@v3
      with:
        version: latest
        args: --timeout=10m
    
    - name: Format check
      run: |
        gofmt -l . | tee format-issues.txt
        test ! -s format-issues.txt

  test:
    name: Tests
    runs-on: ubuntu-latest
    
    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_PASSWORD: test
          POSTGRES_USER: test
          POSTGRES_DB: accumulate_test
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 5432:5432
      
      redis:
        image: redis:7
        options: >-
          --health-cmd "redis-cli ping"
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 6379:6379
    
    strategy:
      matrix:
        test-type: [unit, integration, simulator]
    
    steps:
    - uses: actions/checkout@v4
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: ${{ env.GO_VERSION }}
        cache: true
    
    - name: Run unit tests
      if: matrix.test-type == 'unit'
      run: |
        go test -race -coverprofile=coverage.out -covermode=atomic ./...
        go tool cover -html=coverage.out -o coverage.html
    
    - name: Run integration tests
      if: matrix.test-type == 'integration'
      env:
        ACC_TEST_TIMEOUT: 30m
        POSTGRES_HOST: localhost
        POSTGRES_PORT: 5432
        REDIS_URL: redis://localhost:6379
      run: |
        go test -tags=integration -timeout=30m -v ./test/e2e/...
    
    - name: Run simulator tests
      if: matrix.test-type == 'simulator'
      run: |
        go test -timeout=20m -v ./test/simulator/...
    
    - name: Upload coverage to Codecov
      if: matrix.test-type == 'unit'
      uses: codecov/codecov-action@v3
      with:
        file: ./coverage.out
        flags: unittests
        name: codecov-umbrella

  performance:
    name: Performance Tests
    runs-on: ubuntu-latest
    if: github.event_name == 'push' && (github.ref == 'refs/heads/main' || github.ref == 'refs/heads/develop')
    
    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_PASSWORD: test
          POSTGRES_USER: test
          POSTGRES_DB: accumulate_test
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 5432:5432
    
    steps:
    - uses: actions/checkout@v4
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: ${{ env.GO_VERSION }}
        cache: true
    
    - name: Run benchmarks
      run: |
        go test -bench=. -benchmem -benchtime=10s ./... | tee benchmark.txt
        go install golang.org/x/perf/cmd/benchstat@latest
        benchstat benchmark.txt
    
    - name: Load tests
      run: |
        go build -o accumulated ./cmd/accumulated
        ./accumulated --config=test/configs/ci.toml &
        sleep 10
        go run ./test/cmd/load -server=http://localhost:8080 -duration=60s -transactions=1000
    
    - name: Store benchmark result
      uses: benchmark-action/github-action-benchmark@v1
      with:
        tool: 'go'
        output-file-path: benchmark.txt
        github-token: ${{ secrets.GITHUB_TOKEN }}
        auto-push: true

  security:
    name: Security Scan
    runs-on: ubuntu-latest
    
    steps:
    - uses: actions/checkout@v4
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: ${{ env.GO_VERSION }}
        cache: true
    
    - name: Run Gosec Security Scanner
      uses: securecodewarrior/github-action-gosec@master
      with:
        args: '-fmt sarif -out results.sarif ./...'
    
    - name: Upload SARIF file
      uses: github/codeql-action/upload-sarif@v2
      with:
        sarif_file: results.sarif

  build:
    name: Build
    runs-on: ubuntu-latest
    needs: [validate, test]
    
    steps:
    - uses: actions/checkout@v4
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: ${{ env.GO_VERSION }}
        cache: true
    
    - name: Build binaries
      run: |
        go build -ldflags="-w -s" -o accumulated ./cmd/accumulated
        go build -ldflags="-w -s" -o analyze ./tools/cmd/analyze
    
    - name: Upload artifacts
      uses: actions/upload-artifact@v3
      with:
        name: binaries
        path: |
          accumulated
          analyze
```

### GitHub-Specific Features

```yaml
# Dependency review
dependency-review:
  name: Dependency Review
  runs-on: ubuntu-latest
  if: github.event_name == 'pull_request'
  steps:
    - name: Checkout Repository
      uses: actions/checkout@v4
    - name: Dependency Review
      uses: actions/dependency-review-action@v3

# Auto-merge for dependabot
auto-merge:
  name: Auto-merge Dependabot PRs
  runs-on: ubuntu-latest
  if: github.actor == 'dependabot[bot]'
  needs: [validate, test]
  steps:
    - name: Enable auto-merge for Dependabot PRs
      run: gh pr merge --auto --merge "$PR_URL"
      env:
        PR_URL: ${{github.event.pull_request.html_url}}
        GITHUB_TOKEN: ${{secrets.GITHUB_TOKEN}}
```

## Test Pipeline Design

### Pipeline Stages

```yaml
# Stage 1: Fast Feedback (< 5 minutes)
fast-feedback:
  - lint
  - format-check
  - unit-tests (subset)
  - build-check

# Stage 2: Comprehensive Testing (< 30 minutes)
comprehensive:
  - unit-tests (full)
  - integration-tests
  - simulator-tests
  - security-scan

# Stage 3: Performance & Quality (< 60 minutes)
quality-assurance:
  - benchmark-tests
  - load-tests
  - coverage-analysis
  - dependency-check

# Stage 4: Deployment (manual/automated)
deployment:
  - staging-deploy
  - production-deploy
```

### Parallel Execution Strategy

```yaml
# Example parallel job configuration
test-matrix:
  strategy:
    matrix:
      go-version: ['1.20', '1.21']
      os: [ubuntu-latest, windows-latest, macos-latest]
      test-suite: [unit, integration, simulator]
    fail-fast: false
  
  runs-on: ${{ matrix.os }}
  
  steps:
    - name: Run ${{ matrix.test-suite }} tests on Go ${{ matrix.go-version }}
      run: |
        case "${{ matrix.test-suite }}" in
          unit)
            go test -race ./...
            ;;
          integration)
            go test -tags=integration ./test/e2e/...
            ;;
          simulator)
            go test ./test/simulator/...
            ;;
        esac
```

## Performance Testing in CI

### Benchmark Tracking

```yaml
benchmark-tracking:
  name: Track Performance
  runs-on: ubuntu-latest
  
  steps:
    - name: Run benchmarks
      run: |
        go test -bench=. -benchmem -count=5 ./... | tee benchmark.txt
    
    - name: Compare with baseline
      run: |
        # Download baseline
        curl -o baseline.txt "$BASELINE_URL"
        
        # Compare results
        benchstat baseline.txt benchmark.txt | tee comparison.txt
        
        # Check for regressions
        python3 scripts/check-regression.py comparison.txt
    
    - name: Update baseline
      if: github.ref == 'refs/heads/main'
      run: |
        # Upload new baseline
        curl -X POST -d @benchmark.txt "$BASELINE_UPLOAD_URL"
```

### Performance Regression Detection

```python
# scripts/check-regression.py
import sys
import re

def check_regression(comparison_file):
    with open(comparison_file, 'r') as f:
        content = f.read()
    
    # Parse benchstat output
    regressions = []
    for line in content.split('\n'):
        if '~' in line and '+' in line:
            # Parse performance change
            match = re.search(r'(\+\d+\.\d+%)', line)
            if match:
                change = float(match.group(1)[1:-1])
                if change > 10:  # 10% regression threshold
                    regressions.append(line)
    
    if regressions:
        print("Performance regressions detected:")
        for reg in regressions:
            print(f"  {reg}")
        sys.exit(1)
    
    print("No significant performance regressions detected")

if __name__ == "__main__":
    check_regression(sys.argv[1])
```

### Load Testing Integration

```yaml
load-test-pipeline:
  name: Load Testing
  runs-on: ubuntu-latest
  
  services:
    postgres:
      image: postgres:15
      env:
        POSTGRES_PASSWORD: test
  
  steps:
    - name: Start application
      run: |
        go build -o accumulated ./cmd/accumulated
        ./accumulated --config=test/configs/load-test.toml &
        echo $! > app.pid
        sleep 30  # Wait for startup
    
    - name: Run load tests
      run: |
        # Multiple load test scenarios
        go run ./test/cmd/load \
          -server=http://localhost:8080 \
          -scenario=normal \
          -duration=300s \
          -rps=100
        
        go run ./test/cmd/load \
          -server=http://localhost:8080 \
          -scenario=burst \
          -duration=60s \
          -rps=1000
    
    - name: Collect metrics
      run: |
        # Collect application metrics
        curl http://localhost:8080/metrics > metrics.txt
        
        # Analyze results
        python3 scripts/analyze-load-test.py
    
    - name: Cleanup
      run: |
        kill $(cat app.pid) || true
```

## Artifact Management

### Test Artifacts

```yaml
artifact-collection:
  artifacts:
    reports:
      junit: 
        - "**/test-report.xml"
        - "**/integration-report.xml"
      coverage_report:
        coverage_format: cobertura
        path: coverage.xml
    
    paths:
      - coverage.html
      - benchmark.txt
      - load-test-results.json
      - logs/
      - screenshots/
    
    expire_in: 1 month
    when: always
```

### Binary Artifacts

```yaml
binary-artifacts:
  stage: build
  script:
    - go build -ldflags="-X main.version=$CI_COMMIT_SHA" -o accumulated ./cmd/accumulated
    - go build -ldflags="-X main.version=$CI_COMMIT_SHA" -o analyze ./tools/cmd/analyze
  
  artifacts:
    name: "accumulate-$CI_COMMIT_SHORT_SHA"
    paths:
      - accumulated
      - analyze
    expire_in: 1 week
```

### Docker Images

```yaml
docker-build:
  stage: build
  services:
    - docker:dind
  
  script:
    - docker build -t $CI_REGISTRY_IMAGE:$CI_COMMIT_SHA .
    - docker build -t $CI_REGISTRY_IMAGE:latest .
    - docker push $CI_REGISTRY_IMAGE:$CI_COMMIT_SHA
    - docker push $CI_REGISTRY_IMAGE:latest
  
  only:
    - main
    - develop
```

## Quality Gates

### Coverage Requirements

```yaml
coverage-gate:
  stage: test
  script:
    - go test -coverprofile=coverage.out ./...
    - go tool cover -func=coverage.out | tail -1 | awk '{print $3}' | sed 's/%//' > coverage.txt
    - |
      COVERAGE=$(cat coverage.txt)
      echo "Current coverage: $COVERAGE%"
      if (( $(echo "$COVERAGE < 80" | bc -l) )); then
        echo "Coverage $COVERAGE% is below required 80%"
        exit 1
      fi
```

### Performance Gates

```yaml
performance-gate:
  stage: performance
  script:
    - go test -bench=BenchmarkCritical -benchtime=10s ./... | tee benchmark.txt
    - |
      # Extract performance metrics
      LATENCY=$(grep "BenchmarkCritical" benchmark.txt | awk '{print $3}' | sed 's/ns\/op//')
      
      # Check against threshold (e.g., 1ms = 1000000ns)
      if (( $(echo "$LATENCY > 1000000" | bc -l) )); then
        echo "Performance regression: ${LATENCY}ns > 1000000ns"
        exit 1
      fi
```

### Security Gates

```yaml
security-gate:
  stage: security
  script:
    - gosec -fmt json -out gosec-report.json ./...
    - |
      # Check for high severity issues
      HIGH_ISSUES=$(jq '.Issues | map(select(.severity == "HIGH")) | length' gosec-report.json)
      if [ "$HIGH_ISSUES" -gt 0 ]; then
        echo "Found $HIGH_ISSUES high severity security issues"
        exit 1
      fi
```

## Monitoring and Alerts

### Pipeline Monitoring

```yaml
# Slack notifications
slack-notification:
  stage: notify
  script:
    - |
      if [ "$CI_JOB_STATUS" == "success" ]; then
        MESSAGE="✅ Pipeline succeeded for $CI_COMMIT_REF_NAME"
      else
        MESSAGE="❌ Pipeline failed for $CI_COMMIT_REF_NAME"
      fi
      
      curl -X POST -H 'Content-type: application/json' \
        --data "{\"text\":\"$MESSAGE\"}" \
        $SLACK_WEBHOOK_URL
  when: always
```

### Performance Monitoring

```yaml
performance-alert:
  stage: performance
  script:
    - go test -bench=. -benchmem ./... | tee benchmark.txt
    - python3 scripts/performance-monitor.py benchmark.txt
  
  after_script:
    - |
      if [ -f "performance-alert.txt" ]; then
        curl -X POST -H 'Content-type: application/json' \
          --data "{\"text\":\"⚠️ Performance regression detected: $(cat performance-alert.txt)\"}" \
          $SLACK_WEBHOOK_URL
      fi
```

### Test Failure Analysis

```python
# scripts/analyze-failures.py
import json
import sys

def analyze_test_failures(junit_file):
    with open(junit_file, 'r') as f:
        # Parse JUnit XML
        # Extract failure patterns
        # Generate failure report
        pass

def send_failure_report(report):
    # Send detailed failure analysis
    # Include suggestions for fixes
    # Tag relevant team members
    pass
```

## Best Practices

### 1. Fast Feedback Loop

```yaml
# Optimize for speed in early stages
fast-tests:
  stage: validate
  script:
    # Run only fast tests first
    - go test -short ./...
    # Run critical path tests
    - go test -run "TestCritical.*" ./...
  timeout: 5m
```

### 2. Fail Fast Strategy

```yaml
# Stop on first failure for quick feedback
pipeline:
  fail_fast: true
  
  jobs:
    lint:
      script: golangci-lint run
      allow_failure: false
    
    unit-tests:
      script: go test ./...
      needs: [lint]  # Don't run if lint fails
```

### 3. Resource Optimization

```yaml
# Use appropriate resources
small-tests:
  stage: test
  tags:
    - small-runner
  script:
    - go test ./pkg/...

large-tests:
  stage: test
  tags:
    - large-runner
  script:
    - go test ./test/e2e/...
```

### 4. Environment Consistency

```yaml
# Use consistent environments
.test-environment: &test-env
  variables:
    GO_VERSION: "1.21"
    CGO_ENABLED: 0
    GOOS: linux
    GOARCH: amd64
  before_script:
    - go version
    - go env

unit-tests:
  <<: *test-env
  script:
    - go test ./...
```

### 5. Conditional Execution

```yaml
# Run expensive tests only when needed
load-tests:
  stage: performance
  script:
    - go run ./test/cmd/load
  only:
    changes:
      - "**/*.go"
      - "go.mod"
      - "go.sum"
  except:
    - schedules
```

---

## See Also

- [testing.md](testing.md) - Complete testing guide
- [debugging.md](debugging.md) - Test debugging guide
- [performance-tests.md](performance-tests.md) - Performance testing guide
- [e2e-tests.md](e2e-tests.md) - End-to-end testing guide

*This guide focuses on CI/CD integration. For specific test implementation details, see the related documentation.*
