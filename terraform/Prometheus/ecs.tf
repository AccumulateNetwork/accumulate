resource "aws_ecs_cluster" "prometheus" {
  name = var.name
}

resource "docker_image" "ubuntu" {
  name = "bitnami/prometheus:latest"
}

resource "aws_ecs_task_definition" "prometheus" {
  family                   = "prometheus" # Name of prometheus task
  container_definitions    = <<DEFINITION
  [
    {
      "name": "prometheus",
      "image": "bitnami/prometheus",
      "essential": true,
      "portMappings": [
        {
          "containerPort": 9090,
          "hostPort": 9090
        }
      ],
      "logConfiguration": {
         "logDriver": "awslogs",
         "options": {
            "awslogs-group": "accumulate-prometheus",
            "awslogs-region": "us-east-1",
            "awslogs-stream-prefix": "ecs"
                }
            },
      "mountPoints": [
           {
               "containerPath": "/etc/prometheus",
               "sourceVolume": "config",
               "readOnly": false
           }
       ],
      "memory": 512,
      "cpu": 256,
      "networks": "apod"
    }
   ]
  DEFINITION
  requires_compatibilities = ["FARGATE"] # Stating that we are using ECS Fargate
  network_mode             = "awsvpc"    # Using awsvpc as our network mode as this is a  Fargate requirement
  memory                   = 512         # Specifying the memory our container requires
  cpu                      = 256         # Specifying the CPU our container requires
  volume {
    name     = "config"
    efs_volume_configuration {
      file_system_id          = "${aws_efs_file_system.prometheus.id}"
      root_directory          = "/etc/prometheus"
      transit_encryption      = "ENABLED"
      transit_encryption_port = 2999
      authorization_config {
        access_point_id       = aws_efs_access_point.prometheus.id
        iam                   = "ENABLED"
      }
    }
  }


  execution_role_arn      = "${aws_iam_role.prometheus.arn}"
  task_role_arn           = "${aws_iam_role.prometheus.arn}"
}

resource "aws_ecs_service" "prometheus_service" {
  name            = var.name                             # Name of prometheus service
  cluster         = "${aws_ecs_cluster.prometheus.id}"             # Reference of created Cluster
  task_definition = "${aws_ecs_task_definition.prometheus.arn}" # Reference of task our service will spin up
  launch_type     = "FARGATE"
  desired_count   = 1
  platform_version = "1.4.0"
  depends_on = [aws_alb_listener.prometheus]

  network_configuration {
    subnets          = ["subnet-02c15750cd0264806","subnet-03c42789aa7599f12"]
    assign_public_ip = true # Provide our containers with public IPs
    security_groups  = ["${aws_security_group.prometheus.id}"]
      }

  load_balancer {
    target_group_arn = "${aws_alb_target_group.prometheus.arn}" # Reference our target group
    container_name   = "${aws_ecs_task_definition.prometheus.family}"
    container_port   = 9090 # Specify the container port
  }
  
  service_registries {
    registry_arn = aws_service_discovery_service.prometheus.arn
  }

}


resource "aws_service_discovery_service" "prometheus" {
  name        = "prometheus"
  description = var.name

  dns_config {
    dns_records {
      ttl  = 0
      type = "A"
    }

    namespace_id   = aws_service_discovery_private_dns_namespace.prometheus.id
    routing_policy = "MULTIVALUE"
  }

  health_check_custom_config {
    failure_threshold = 1
  }
}