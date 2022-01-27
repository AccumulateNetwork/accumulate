locals {
  base_container = {
    image: var.image,
    essential: true
    portMappings: [
      { containerPort: 8080  }, // Website
      { containerPort: 26656 }, // Tendermint P2P
      { containerPort: 26657 }, // Tendermint JSON-RPC
      { containerPort: 26658 }, // Tendermint gRPC
      { containerPort: 26660 }, // Accumulate JSON-RPC
      { containerPort: 26662 }, // Prometheus
    ]
    memory: 512
    cpu: 256
    logConfiguration: {
      logDriver: "awslogs"
      options: {
        awslogs-group: aws_cloudwatch_log_group.devnet_log.name
        awslogs-region: "us-east-1"
        awslogs-stream-prefix: "ecs"
      }
    }
    mountPoints: [
      {
        containerPath: "/mnt/efs/node"
        sourceVolume: "accumulate-${var.name}-storage"
        readOnly: false
      }
    ]
  }

  init = {
    containers: [
      merge(local.base_container, {
        name: "init"
        command: [
          "init", "devnet",
          "--docker",
          "--work-dir=/mnt/efs/node",
          "--dns-suffix=.accumulate-${var.name}",
          "--bvns=${var.bvns}",
          "--validators=${var.validators}",
          "--followers=0",
        ]
      })
    ]
  }

  subnets = flatten([
    for subnet in concat(["dn"], [for n in range(0, var.bvns) : "bvn${n}" ]) : [
      for n in range(0, var.validators) : {
        subnet: subnet
        name: "${subnet}-${n}"
        containers: [
          merge({
            name: "${subnet}-${n}"
            command: ["run", "-w", "/mnt/efs/node/${subnet}/Node${n}"]
          }, local.base_container)
        ]
      }
    ]
  ])
}

resource "aws_cloudwatch_log_group" "devnet_log" {
  name = "accumulate-${var.name}"
}

resource "aws_ecs_cluster" "dev_cluster" {
  name = "accumulate-${var.name}"
}

resource "aws_ecs_task_definition" "accumulate-init" {
  family                   = "accumulate-${var.name}-init"
  container_definitions    = jsonencode(local.init.containers)
  requires_compatibilities = ["FARGATE"] # Stating that we are using ECS Fargate
  network_mode             = "awsvpc"    # Using awsvpc as our network mode as this is a  Fargate requirement
  memory                   = 512         # Specifying the memory our container requires
  cpu                      = 256         # Specifying the CPU our container requires
  volume {
    name = "accumulate-${var.name}-storage"
    efs_volume_configuration {
      file_system_id          = "${aws_efs_file_system.devnet.id}"
      root_directory          = "/mnt/efs/node"
      transit_encryption      = "ENABLED"
      transit_encryption_port = 2999
      authorization_config {
        access_point_id = aws_efs_access_point.devnet.id
        iam = "ENABLED"
      }
    }
  }
  execution_role_arn       = "${aws_iam_role.ecsTaskExecutionRole_1.arn}"
  task_role_arn            = "${aws_iam_role.ecsTaskExecutionRole_1.arn}"
}

resource "aws_ecs_task_definition" "accumulate-node" {
  for_each                 = { for subnet in local.subnets : subnet.name => subnet }
  family                   = "accumulate-${var.name}-${each.value.name}"
  container_definitions    = jsonencode(each.value.containers)
  requires_compatibilities = ["FARGATE"] # Stating that we are using ECS Fargate
  network_mode             = "awsvpc"    # Using awsvpc as our network mode as this is a  Fargate requirement
  memory                   = 512         # Specifying the memory our container requires
  cpu                      = 256         # Specifying the CPU our container requires
  volume {
    name = "accumulate-${var.name}-storage"
    efs_volume_configuration {
      file_system_id          = "${aws_efs_file_system.devnet.id}"
      root_directory          = "/mnt/efs/node"
      transit_encryption      = "ENABLED"
      transit_encryption_port = 2999
      authorization_config {
        access_point_id = aws_efs_access_point.devnet.id
        iam = "ENABLED"
      }
    }
  }
  execution_role_arn       = "${aws_iam_role.ecsTaskExecutionRole_1.arn}"
  task_role_arn            = "${aws_iam_role.ecsTaskExecutionRole_1.arn}"
}

resource "aws_service_discovery_service" "accumulate-node" {
  for_each = { for subnet in local.subnets : subnet.name => subnet }
  name     = each.value.name

  dns_config {
    namespace_id   = aws_service_discovery_private_dns_namespace.devnet.id
    routing_policy = "MULTIVALUE"

    dns_records {
      ttl  = 10
      type = "A"
    }
  }

  health_check_custom_config {
    failure_threshold = 1
  }
}

resource "aws_ecs_service" "accumulate-node" {
  for_each         = { for subnet in local.subnets : subnet.name => subnet }
  name             = each.value.name
  cluster          = "${aws_ecs_cluster.dev_cluster.id}"
  task_definition  = "${aws_ecs_task_definition.accumulate-node[each.key].arn}"
  launch_type      = "FARGATE"
  desired_count    = 1
  platform_version = "1.4.0"
  depends_on       = [aws_alb_listener.dev_listener]

  load_balancer {
    target_group_arn = "${aws_alb_target_group.dev_target.arn}" # Reference our target group
    container_name   = "${aws_ecs_task_definition.accumulate-node[each.key].family}"
    container_port   = 26660
  }

  service_registries {
    registry_arn = aws_service_discovery_service.accumulate-node[each.key].arn
    container_name = each.value.name
  }

  network_configuration {
    subnets          = ["${aws_subnet.dev_pubsub_a.id}","${aws_subnet.dev_pubsub_b.id}"]
    assign_public_ip = true # Provide our containers with public IPs
    security_groups  = ["${aws_security_group.devnet.id}"]
  }
}