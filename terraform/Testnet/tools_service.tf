resource "aws_ecs_cluster" "test_cluster" {
  name = "accumulate-test-cluster" # Name of cluster
}




data "aws_ecs_task_definition" "tool_service" {
  task_definition = "${aws_ecs_task_definition.tool_service[count.index].family}"
  count           = 4
}

resource "aws_ecs_task_definition" "tool_service" {
  count                    = 4
  depends_on               = [aws_ecs_cluster.test_cluster]
  family                   = "tool_service" # Name of first task
  container_definitions    = <<DEFINITION
  [
    {
      "name": "tool_service",
      "image": "registry.gitlab.com/accumulatenetwork/accumulate/accumulated:stable",
      "essential": true,
      "portMappings": [{"containerPort": 80}],
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
      "command": ["init", "devnet", "--work-dir", "/mnt/efs/node", "--docker", "--bvns=3", "--followers=0", "--validators=4", "--dns-suffix", ".accumulate-testnet"],     
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
  requires_compatibilities    = ["FARGATE"] # Stating that we are using ECS Fargate
  network_mode                = "awsvpc"    # Using awsvpc as our network mode as this is a  Fargate requirement
  memory                      = 512    # Specifying the memory our container requires
  cpu                         = 256     # Specifying the CPU our container requires
  volume {
    name     = "efs_temp"
    efs_volume_configuration {
      file_system_id = "${aws_efs_file_system.testnet.id}"
      root_directory = "/mnt/efs/node"
      transit_encryption      = "ENABLED"
      transit_encryption_port = 2999
      authorization_config {
        access_point_id       = aws_efs_access_point.testnet.id
        iam                  = "ENABLED"

      }
    }
  }
  execution_role_arn          = "${aws_iam_role.ecsTaskExecutionRole.arn}"
  task_role_arn               = "${aws_iam_role.ecsTaskExecutionRole.arn}"
}


# resource "aws_ecs_service" "tool_service" {
#   name             = "tool-service"              
#   cluster          = "${aws_ecs_cluster.test_cluster.id}"
#   task_definition  = "${aws_ecs_task_definition.tool_service.arn}"
#   launch_type      = "FARGATE"
#   desired_count    = 4
#   platform_version = "1.4.0"


#   network_configuration {
#     subnets          = ["subnet-02c15750cd0264806","subnet-03c42789aa7599f12"]
#     assign_public_ip = true # Provide our containers with public IPs
#     security_groups  = ["${aws_security_group.tool_service.id}"]
#       }

#   #  depends_on = [aws_alb_listener.listener]   
# }

# resource "aws_cloudwatch_event_rule" "scheduled_task" {
#   name                = "tool_service_scheduled_task"
#   description         = "Run task and stop when exit code is 0"
#   schedule_expression = "cron(* 9 * ? *)"
# }

# resource "aws_cloudwatch_event_target" "scheduled_task" {
#   target_id = "scheduled_task_target"
#   rule      = "${aws_cloudwatch_event_rule.scheduled_task.name}"
#   arn       = "${aws_ecs_cluster.test_cluster.id}"
#   role_arn  = "${aws_iam_role.scheduled_task_cloudwatch.arn}"


# ecs_target {
#     task_count          =  4
#     task_definition_arn = "${aws_ecs_task_definition.tool_service.arn}"
#     launch_type         ="FARGATE"
#     platform_version    ="1.4.0"


    # network_configuration {
    #   assign_public_ip = true
    #   security_groups= ["${aws_security_group.tool_service.id}"]
    #   subnets = ["subnet-02c15750cd0264806","subnet-03c42789aa7599f12"]
    # }
#   }
# } 



