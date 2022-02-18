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
            name = "tool-service"
            command = [ "init", "devnet", "--work-dir", "/mnt/efs/node", "--docker", "--bvns=3", "--followers=0", "--validators=4", "--dns-suffix", ".accumulate-testnet" ]

            resources {
                limits = {
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

resource "kubernetes_service" "testnet_service" {
  metadata {
    name = "accumulate-tool"
  }
  spec {
      selector = {
          test= "FirstApp"
      }
      port {
        port     = 80
        target_port = 80
      }
      type = "LoadBalancer"
  }
}