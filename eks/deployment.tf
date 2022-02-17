resource "kubernetes_deployment" "tool_service" {
  metadata {
      name = "testnet-tools"
      labels = {
        "test" = "FirstApp"
      }
  }

  spec {
    replicas = 1

    selector {
        match_labels = {
          "test" = "FirstApp"
        }
    }

    template {
      metadata {
          labels = {
              "test" = "FirstApp"
          }
      }

      spec {
        container {
            image = "registry.gitlab.com/accumulatenetwork/accumulate/accumulated:stable"
            name = "tool_service"

            resources {
                limits {
                    cpu    = "0.5"
                    memory = "512Mi"
                }
                requests = {
                  "cpu" = "250m"
                  "memory" = "50Mi"
                }
            }
        }
      }
    }
  }
}