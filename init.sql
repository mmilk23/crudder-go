-- Criação do banco de dados
-- CREATE DATABASE IF NOT EXISTS crudder_db_test;

-- Uso do banco de dados
USE crudder_db_test;

-- Criação de um usuário e definição de senha
CREATE USER IF NOT EXISTS 'crudder_user'@'%' IDENTIFIED BY 'crudder_p455w0rd';

-- Conceder permissões para o usuário
GRANT ALL PRIVILEGES ON crudder_db_test.* TO 'crudder_user'@'%';

-- Criação das tabelas
CREATE TABLE `roles` (
  `role_id` int(11) AUTO_INCREMENT NOT NULL,
  `role` varchar(100) NOT NULL,
  PRIMARY KEY (`role_id`),
  UNIQUE KEY `role` (`role`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE `users` (
  `user_id` int(11) AUTO_INCREMENT NOT NULL,
  `username` varchar(100) NOT NULL,
  `pwd` varchar(50) NOT NULL,
  PRIMARY KEY (`user_id`),
  UNIQUE KEY `username` (`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

CREATE TABLE `user_roles` (
  `user_id` int(11) NOT NULL,
  `role_id` int(11) NOT NULL,
  UNIQUE KEY `user_id` (`user_id`, `role_id`),
  KEY `user_id_2` (`user_id`),
  KEY `role_id` (`role_id`),
  CONSTRAINT `user_roles_ibfk_1` FOREIGN KEY (`user_id`) REFERENCES `users` (`user_id`),
  CONSTRAINT `user_roles_ibfk_2` FOREIGN KEY (`role_id`) REFERENCES `roles` (`role_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

-- Inserir dados nas tabelas
INSERT INTO users (username, pwd) VALUES
    ('user1', '123123'),
    ('user2', '123123'),
    ('user3', '123123');

INSERT INTO roles (`role`) VALUES
    ('role1'),
    ('role2');

INSERT INTO user_roles (user_id, role_id) VALUES
    (1, 1),
    (1, 2),
    (2, 1);
