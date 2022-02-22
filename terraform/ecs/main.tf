resource "aws_ecs_cluster" "dev_cluster" {
    name = "accumulate-dev-cluster"
}

data "aws_ecs_task_definition" "dev_tools" {
  task_definition = "${aws_ecs_task_definition.dev_tools.family}"
}


resource "aws_ecs_task_definition" "dev_tools" {
  family                   = "dev_tools" # Name of first task
  container_definitions    = <<DEFINITION
  [
    {
      "name": "dev_tools",
      "image": "public.ecr.aws/k4z4t6h4/accumulate/accumulated:latest",
      "essential": true,
      "portMappings": [{"containerPort": 26660}],
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
      "command": ["init", "devnet", "--work-dir", "/mnt/efs/node", "--docker", "--bvns=3", "--followers=0", "--validators=4"],
      "mountPoints": [
          {
              "containerPath": "/mnt/efs/node",
              "sourceVolume": "efs_temp"
          }
      ]                 
    }
  ]
DEFINITION
  #requires_compatibilities    = ["EC2"] # Stating that we are using ECS Fargate
  network_mode                = "bridge"    # Using awsvpc as our network mode as this is a  Fargate requirement
  memory                      = 512    # Specifying the memory our container requires
  cpu                         = 256    # Specifying the CPU our container requires
  volume {
    name     = "efs_temp"
    efs_volume_configuration {
      file_system_id          = "${aws_efs_file_system.devnet.id}"
      root_directory          = "/mnt/efs/node"
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

resource "aws_ecs_service" "devnet_service" {
  name             = "devnet-service"              
  cluster          = "${aws_ecs_cluster.dev_cluster.id}"
  task_definition  = "${aws_ecs_task_definition.dev_tools.arn}"
  launch_type      = "EC2"
  desired_count    = 1
  


  # load_balancer {
  #   target_group_arn = "${aws_alb_target_group.dev_target.arn}" # Reference our target group
  #   container_name   = "dev_tools"
  #   container_port   = 9000
  # }

    depends_on = [aws_alb_listener.dev_listener]
}