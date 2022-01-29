resource "aws_ecs_cluster" "dev_cluster" {
    name = "accumulate-dev-cluster"
}

data "aws_ecs_task_definition" "init" {
  task_definition = "${aws_ecs_task_definition.init.family}"
}


resource "aws_ecs_task_definition" "init" {

  depends_on               = [aws_ecs_cluster.dev_cluster]
  family                   = "accumulate-devnet-init" # Name of first task
  container_definitions    = <<DEFINITION
  [
    {
      "name": "accumulate-devnet-init",
      "image": "registry.gitlab.com/accumulatenetwork/accumulate/accumulated:develop",
      "essential": true,
      "memory": 512,
      "cpu": 256,
      "logConfiguration": {
         "logDriver": "awslogs",
         "options": {
            "awslogs-group": "accumulate-devnet-log",
            "awslogs-region": "us-east-1",
            "awslogs-stream-prefix": "ecs"
                }
            },
      "entryPoint": ["sh"],
      "command": ["mount", "&&", "accumulated", "init", "devnet", "--work-dir", "/mnt/efs/node", "--docker", "--bvns=3", "--followers=0", "--validators=4", "--dns-suffix=.accumulate-devnet", "--reset", "--log-levels=error;accumulate=debug;executor=debug;governor=debug"],
      "mountPoints": [
          {
              "containerPath": "/mnt/efs/node",
              "sourceVolume": "efs_temp"
          }
      ]
    }
  ]
DEFINITION
  requires_compatibilities    = ["FARGATE"] # Stating that we are using ECS Fargate
  network_mode                = "awsvpc"    # Using awsvpc as our network mode as this is a  Fargate requirement
  memory                      = 512    # Specifying the memory our container requires
  cpu                         = 256    # Specifying the CPU our container requires
  volume {
    name     = "efs_temp"
    efs_volume_configuration {
      file_system_id          = "${aws_efs_file_system.devnet.id}"
      transit_encryption      = "ENABLED"
      transit_encryption_port = 2999
      authorization_config {
        access_point_id       = aws_efs_access_point.devnet.id
         iam                  = "ENABLED"

      }
    }
  }
  execution_role_arn       = "${aws_iam_role.ecsTaskExecutionRole_1.arn}"
  task_role_arn            = "${aws_iam_role.ecsTaskExecutionRole_1.arn}"
}
