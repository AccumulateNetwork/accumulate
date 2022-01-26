data "aws_ecs_task_definition" "dn-0" {
  task_definition = "${aws_ecs_task_definition.dn-0.family}"
}

resource "aws_ecs_task_definition" "dn-0" {
  family = "dn-0"
  container_definitions = <<DEFINITION
[

   {
      "name": "dn-0",
      "image": "registry.gitlab.com/accumulatenetwork/accumulate/accumulated:stable",
      "essential": true,
      "portMappings": [{"containerPort": 3000}],
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
      "command": ["run", "-w", "/mnt/efs/node/dn/Node0"],
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
      file_system_id = "${aws_efs_file_system.testnet.id}"
      root_directory = "/mnt/efs/node"
      transit_encryption      = "ENABLED"
      transit_encryption_port = 2999
      authorization_config {
        access_point_id = aws_efs_access_point.testnet.id
        iam = "ENABLED"
      }
    }
  }

  execution_role_arn       = "${aws_iam_role.ecsTaskExecutionRole.arn}"
  task_role_arn            = "${aws_iam_role.ecsTaskExecutionRole.arn}"

}

  resource "aws_ecs_service" "dn-0" {
  name            = "dn-0"              
  cluster         = "${aws_ecs_cluster.test_cluster.id}"
  task_definition = "${aws_ecs_task_definition.dn-0.arn}"
  launch_type     = "FARGATE"
  desired_count   = 1
  platform_version = "1.4.0"


  load_balancer {
    target_group_arn = "${aws_alb_target_group.target_group.arn}" # Reference our target group
    container_name   = "${aws_ecs_task_definition.dn-0.family}"
    container_port   = 3000
  }

  service_registries {
      registry_arn = aws_service_discovery_service.simple-stack-dn0.arn
      container_name = "dn-0"
  }

  network_configuration {
    subnets          = ["subnet-02c15750cd0264806","subnet-03c42789aa7599f12"]
    assign_public_ip = true # Provide our containers with public IPs
    security_groups  = ["${aws_security_group.testnet.id}"]
      }

  depends_on = [aws_alb_listener.listener]
}

resource "aws_service_discovery_service" "simple-stack-dn0" {
  name = "dn-0"

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