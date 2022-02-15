data "aws_ecs_task_definition" "bvn0-1" {
  task_definition = "${aws_ecs_task_definition.bvn0-1.family}"
}

resource "aws_ecs_task_definition" "bvn0-1" {
  family = "bvn0-1"
  container_definitions = <<DEFINITION
[

   {
      "name": "bvn0-1",
      "image": "registry.gitlab.com/accumulatenetwork/accumulate/accumulated:develop",
      "essential": true,
      "portMappings": [{"containerPort": 26660}],
      "memory": 512,
      "cpu": 256,
      "logConfiguration": {
         "logDriver": "awslogs",
         "options": {
            "awslogs-group": "accumulate-testnet-log",
            "awslogs-region": "us-east-1",
            "awslogs-stream-prefix": "ecs"
                }
            },
      "command": ["run", "-w", "/mnt/efs/node/bvn0/Node1"],
      "mountPoints": [
           {
               "containerPath": "/mnt/efs/node",
               "sourceVolume": "efs_temp",
               "readOnly": false
           }
       ]
       
    }
]
DEFINITION
  requires_compatibilities = ["FARGATE"] # Stating that we are using ECS Fargate
  network_mode             = "awsvpc"    # Using awsvpc as our network mode as this is a  Fargate requirement
  memory                   = 512   # Specifying the memory our container requires
  cpu                      = 256      # Specifying the CPU our container requires
  volume {
    name     = "efs_temp"
    efs_volume_configuration {
      file_system_id = "fs-0ad0a2434d1d42e08"
      root_directory = "/mnt/efs/node"
      transit_encryption      = "ENABLED"
      transit_encryption_port = 2999
      authorization_config {
        access_point_id = "fsap-0f807646e2d6fe23c"
        iam = "ENABLED"
      }
    }
  }

  execution_role_arn       = "arn:aws:iam::018508593216:role/ecsTaskExecutionRole"
  task_role_arn            = "arn:aws:iam::018508593216:role/ecsTaskExecutionRole"

}

  resource "aws_ecs_service" "bvn0-1" {
  name            = "bvn0-1"              
  cluster         = "accumulate-test-cluster"
  task_definition = "${aws_ecs_task_definition.bvn0-1.arn}"
  launch_type     = "FARGATE"
  desired_count   = 1
  platform_version = "1.4.0"


  load_balancer {
    target_group_arn = "${aws_alb_target_group.target_group.arn}" # Reference our target group
    container_name   = "${aws_ecs_task_definition.bvn0-1.family}"
    container_port   = 26660
  }

  service_registries {
      registry_arn = aws_service_discovery_service.simple-stack-bvn01.arn
      container_name = "bvn0-1"
  }

  network_configuration {
    subnets          = ["subnet-02c15750cd0264806","subnet-03c42789aa7599f12"]
    assign_public_ip = true # Provide our containers with public IPs
    security_groups  = ["sg-05956d528f4054b52"]
      }

  depends_on = [aws_alb_listener.listener]
}

resource "aws_service_discovery_service" "simple-stack-bvn01" {
  name = "bvn0-1"

  dns_config {
    namespace_id = aws_service_discovery_private_dns_namespace.simple-stack.id

    dns_records {
      ttl  = 10
      type = "A"
    }


    routing_policy = "MULTIVALUE"
  }

  health_check_custom_config {
    failure_threshold = 1
  }
}
