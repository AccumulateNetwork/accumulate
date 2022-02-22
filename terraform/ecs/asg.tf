resource "aws_launch_configuration" "ecs_launch_config" {
    name                 = "accumulate-ecs-test"
    image_id             = "ami-0b49a4a6e8e22fa16"
    iam_instance_profile = aws_iam_instance_profile.ecs_agent.name
    security_groups      = [aws_security_group.dev_tools.id]
    user_data            = "#!/bin/bash\necho ECS_CLUSTER=accumulate-dev-cluster >> /etc/ecs/ecs.config"
    instance_type        = "t4g.medium"
    associate_public_ip_address = true
    key_name = var.key_name
}

resource "aws_autoscaling_group" "failure_analysis_ecs_asg" {
    name                      = "asg"
    vpc_zone_identifier       = [aws_subnet.dev_pubsub_a.id, aws_subnet.dev_pubsub_b.id]
    launch_configuration      = aws_launch_configuration.ecs_launch_config.name

    desired_capacity          = 1
    min_size                  = 1
    max_size                  = 1
    health_check_grace_period = 300
    health_check_type         = "EC2"
    target_group_arns         = [aws_alb_target_group.dev_target.arn]
    protect_from_scale_in     = true
    lifecycle {
      create_before_destroy   = true
    }
}