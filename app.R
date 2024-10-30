# Required libraries
library(tidyverse)
library(shiny)
library(dplyr)
library(ggplot2)
library(DT)

# Load your data
watching_data <- read.csv("watching_data.csv") %>%
  mutate(date = as.Date(date))
service_costs <- read.csv("service_costs.csv")

# UI
ui <- fluidPage(
  titlePanel("Streaming Service Analytics"),
  
  sidebarLayout(
    sidebarPanel(
      dateRangeInput("date_range",
                     "Select Date Range",
                     start = min(watching_data$date),
                     end = today()),
      
      # Modify the existing checkboxGroupInput
      checkboxGroupInput("services",
                  "Select Services",
                  choices = unique(watching_data$service),
                  selected = unique(watching_data$service)),
                  
      
      hr(),
      
      # Summary stats
      textOutput("total_hours"),
      textOutput("total_cost"),
      textOutput("cost_per_hour")
    ),
    
    mainPanel(
      tabsetPanel(
        tabPanel("Overview",
                 plotOutput("usage_plot"),
                 plotOutput("cost_efficiency_plot")),
        tabPanel("Details",
                 DT::dataTableOutput("content_table"))
      )
    )
  )
)

# Server
server <- function(input, output, session) {
  # Reactive data filtering
  filtered_data <- reactive({
    watching_data %>%
      filter(date >= input$date_range[1],
             date <= input$date_range[2],
             service %in% input$services)
  })
  
  # Calculate metrics
  metrics <- reactive({
    watch_data <- filtered_data()
    cost_data <- service_costs %>%
      filter(service %in% input$services)
    
    total_hours <- sum(watch_data$watch_time) / 60  # Assuming watch_time is in minutes
    total_cost <- sum(cost_data$monthly_cost) * 
      (as.numeric(diff(input$date_range)) / 30)  # Prorated cost
    
    list(
      total_hours = total_hours,
      total_cost = total_cost,
      cost_per_hour = total_cost / total_hours
    )
  })
  
  # Outputs
  output$total_hours <- renderText({
    paste("Total Hours Watched:", round(metrics()$total_hours, 1))
  })
  
  output$total_cost <- renderText({
    sprintf("Total Cost: $%.2f", metrics()$total_cost)
  })
  
  output$cost_per_hour <- renderText({
    sprintf("Cost per Hour: $%.2f", metrics()$cost_per_hour)
  })
  
  output$usage_plot <- renderPlot({
    filtered_data() %>%
      group_by(service, date = floor_date(date, "month")) %>%
      summarise(hours = sum(watch_time) / 60) %>%
      ggplot(aes(x = date, y = hours, fill = service)) +
      geom_col() +
      theme_minimal() +
      labs(title = "Monthly Viewing Hours by Service")
  })
  
  output$cost_efficiency_plot <- renderPlot({
    filtered_data() %>%
      group_by(service) %>%
      summarise(
        total_hours = sum(watch_time) / 60,
        cost = sum(service_costs$monthly_cost[
          service_costs$service == first(service)
        ])
      ) %>%
      mutate(cost_per_hour = cost / total_hours) %>%
      ggplot(aes(x = reorder(service, cost_per_hour), y = cost_per_hour)) +
      geom_col() +
      coord_flip() +
      theme_minimal() +
      labs(title = "Cost per Hour by Service",
           x = "Service",
           y = "Cost per Hour ($)")
  })
  
  output$content_table <- DT::renderDataTable({
    filtered_data() %>%
      select(date, service, title, watch_time) %>%
      mutate(watch_time = sprintf("%.2f", watch_time / 60)) %>%
      arrange(desc(date))
  })
}

# Run the app
shinyApp(ui = ui, server = server)